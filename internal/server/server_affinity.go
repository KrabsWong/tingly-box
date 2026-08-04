package server

import (
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/protocolserver"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// updateAffinityMessageID updates the affinity entry with the latest message ID
func (s *Server) updateAffinityMessageID(c *gin.Context, rule *typ.Rule, messageID string) {
	if !rule.AffinityEnabled() || messageID == "" {
		return
	}

	// Use the partition-scoped affinity key set at selection time; fall back
	// to the bare session for callers that predate the scoped keys.
	key, exists := c.Get(protocolserver.ContextKeyAffinityKey)
	if !exists {
		key, exists = c.Get(protocolserver.ContextKeySessionID)
		if !exists {
			return
		}
	}

	s.affinityStore.UpdateMessageID(rule.UUID, key.(string), messageID)
	logrus.Debugf("[affinity] updated message ID %s for affinity key %s, rule %s", messageID, key.(string), rule.UUID)
}
