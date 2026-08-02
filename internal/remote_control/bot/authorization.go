package bot

import (
	"context"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/remote/access"
)

type AccessStore interface {
	access.FactSource
	GetCapability(context.Context, string, access.CapabilityName) (access.BotCapability, bool, error)
	DiscoverDirectChat(context.Context, string, string, string) (access.DirectChat, error)
	GetDirectChat(context.Context, string, string) (access.DirectChat, bool, error)
	PairDirectChat(context.Context, string, string, string) error
	DiscoverGroup(context.Context, string, string, string, string) (access.Group, error)
	UpsertActor(context.Context, access.Actor) (access.Actor, error)
}

func isPairingBootstrap(msg imbot.Message) bool {
	return msg.IsDirectMessage() && msg.IsTextContent() && func() bool {
		t := strings.TrimSpace(msg.GetText())
		return t == "/bind" || strings.HasPrefix(t, "/bind ") || strings.HasPrefix(t, "/bind\t")
	}()
}

func inboundAction(msg imbot.Message, pendingCapability access.CapabilityName, pendingAction access.ActionName) (access.CapabilityName, access.ActionName) {
	if msg.IsCallback() && msg.Payload.Name() == "perm" || pendingAction != "" {
		if pendingCapability == "" {
			pendingCapability = access.CapabilityRemoteControl
		}
		if pendingAction == "" {
			pendingAction = access.ActionRemoteControlApprove
		}
		return pendingCapability, pendingAction
	}
	if msg.IsMediaContent() {
		return access.CapabilityRemoteControl, access.ActionRemoteControlPrivileged
	}
	text := strings.ToLower(strings.TrimSpace(msg.GetText()))
	for _, prefix := range []string{"/cd", "/bind-project", "/project switch", "/shell", "/bash"} {
		if text == prefix || strings.HasPrefix(text, prefix+" ") {
			return access.CapabilityRemoteControl, access.ActionRemoteControlPrivileged
		}
	}
	return access.CapabilityRemoteControl, access.ActionRemoteControlStart
}

// authorizationGate is the one production inbound seam for text, callbacks,
// and files. A denied message is claimed here and cannot reach a Capability.
func authorizationGate(store AccessStore, authorizer access.Authorizer, legacyChats ChatStoreInterface, requirePairing bool, pendingForChat func(string) (access.CapabilityName, access.ActionName)) OnMessage {
	return func(msg imbot.Message, platform imbot.Platform, botUUID string) bool {
		if store == nil || authorizer == nil {
			return false
		}
		ctx := context.Background()
		externalTarget := msg.GetReplyTarget()
		externalActor := strings.TrimSpace(msg.Sender.ID)
		if externalTarget == "" || externalActor == "" {
			return true
		}
		actor, err := store.UpsertActor(ctx, access.Actor{BotUUID: botUUID, Platform: string(platform), ExternalActorID: externalActor, DisplayName: msg.Sender.DisplayName})
		if err != nil {
			logrus.WithError(err).Warn("bot access: resolve actor")
			return true
		}
		var target access.TargetRef
		if msg.IsGroupMessage() {
			group, err := store.DiscoverGroup(ctx, botUUID, string(platform), externalTarget, msg.Recipient.DisplayName)
			if err != nil {
				logrus.WithError(err).Warn("bot access: resolve group")
				return true
			}
			target = access.TargetRef{Kind: access.TargetGroup, ID: group.ID}
		} else if msg.IsDirectMessage() {
			chat, err := store.DiscoverDirectChat(ctx, botUUID, string(platform), externalTarget)
			if err != nil {
				logrus.WithError(err).Warn("bot access: resolve direct chat")
				return true
			}
			target = access.TargetRef{Kind: access.TargetDirectChat, ID: chat.ID}
			if isPairingBootstrap(msg) {
				return false
			}
			if chat.PeerActorID == "" {
				legacyPaired := legacyChats != nil && legacyChats.IsChatPaired(externalTarget, botUUID)
				// Pairing-disabled Bots historically accepted their direct peer
				// immediately. Materialize that trust as explicit policy instead
				// of leaving the new authorizer stuck on discovered-chat deny.
				if legacyPaired || !requirePairing {
					if err := store.PairDirectChat(ctx, chat.ID, externalActor, msg.Sender.DisplayName); err != nil {
						logrus.WithError(err).Warn("bot access: establish direct peer")
						return true
					}
				}
			}
		} else {
			return true
		}
		var pendingCapability access.CapabilityName
		var pendingAction access.ActionName
		if pendingForChat != nil {
			pendingCapability, pendingAction = pendingForChat(externalTarget)
		}
		capability, action := inboundAction(msg, pendingCapability, pendingAction)
		decision := authorizer.Evaluate(ctx, access.AuthorizationRequest{BotUUID: botUUID, Target: target, ActorID: actor.ID, Capability: capability, Action: action})
		logrus.WithFields(logrus.Fields{"event": "bot.authorization.decision", "bot_uuid": botUUID, "capability": capability, "action": action, "target_kind": target.Kind, "target_id": target.ID, "actor_id": actor.ID, "allowed": decision.Allowed, "failed_gate": decision.FailedGate, "reason": decision.Reason}).Debug("bot authorization decision")
		return !decision.Allowed
	}
}
