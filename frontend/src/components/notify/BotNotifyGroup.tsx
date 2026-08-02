import {ContentCopy as CopyIcon, Edit as CustomIcon, Close as CloseIcon, Code as CodeIcon, Refresh as RefreshIcon, Block as BlockIcon, Delete as DeleteIcon} from '@/components/icons';
import {api} from '@/services/api';
import {notify} from '@/utils/notify';
import {capabilityEnabled, isPairingRequired} from '@/types/bot';
import type {BotChat, BotSettings} from '@/types/bot';
import {fontMono} from '@/theme/fonts';
import NotifyTestDialog from '@/components/notify/NotifyTestDialog';
import ConfirmDialog from '@/components/ConfirmDialog';
import {CHAT_CAPABILITIES} from '@/components/notify/chatCapabilities';
import useChatProbe, {type ChatCapability, type ChatProbeResult} from '@/components/notify/useChatProbe';
import {ApiEntryNode, ArrowNode, ChatNode, ImBotNode, NodeContainer, getInactiveHatchSx, graphRowStyles} from '@/components/nodes';
import {
    Alert,
    Box,
    Button,
    Chip,
    CircularProgress,
    Collapse,
    IconButton,
    Link,
    Stack,
    Switch,
    Tooltip,
    Typography,
} from '@mui/material';
import {useCallback, useEffect, useState} from 'react';
import {useTranslation} from 'react-i18next';

// BotNotifyGroup is one bot's panel on the IM Notify page: a header (name +
// platform + the enabled switch that governs whether this bot can be driven)
// over an ALWAYS-EXPANDED list of the chats it can reach. Each chat row is a
// CAPABILITY PROBE BENCH — a row of one-click buttons (Notify / Confirm active,
// Choose / Ask gated, Custom → free-form dialog) that exercise each chat
// capability end-to-end and show a probe-style verdict inline, exactly as the
// model-routing probe does for providers (see components/probe/). This answers
// the operator's real question — "do my bot's chat capabilities actually work?"
// — not "can I compose one custom message?" (ux-principles #1/#5/#11).
//
// The chats are fetched eagerly when the bot is enabled, not behind a button.
// A disabled bot has no channel in the registry, so /chats would answer an
// empty list with running:false — we skip the round trip entirely and let the
// graph's empty fork explain why (the "Bot is off" leaf text).
export interface BotNotifyGroupProps {
    bot: BotSettings;
    onToggle: (uuid: string, enabled: boolean) => void;
    isToggling?: boolean;
}

// formatLatency mirrors probe/runProbe.ts: "850ms" / "1.2s".
const formatLatency = (ms: number): string => (ms < 1000 ? `${ms}ms` : `${(ms / 1000).toFixed(1)}s`);

// One-glance verdict label + severity for a probe result, mirroring the probe
// feature's StatusBar mapping of outcome → label.
const verdict = (r: ChatProbeResult): {label: string; severity: 'success' | 'error' | 'warning' | 'info'} => {
    switch (r.status) {
        case 'delivered': return {label: `Delivered · ${formatLatency(r.latencyMs)}`, severity: 'success'};
        case 'answered': return {label: `Answered: ${String(r.decision?.selected ?? '?')} · ${formatLatency(r.latencyMs)}`, severity: 'success'};
        case 'cancelled': return {label: `Cancelled · ${formatLatency(r.latencyMs)}`, severity: 'warning'};
        case 'timed-out': return {label: `Timed out · ${formatLatency(r.latencyMs)}`, severity: 'warning'};
        case 'expired': return {label: `Expired · ${formatLatency(r.latencyMs)}`, severity: 'warning'};
        default: return {label: r.error ? `Failed: ${r.error}` : 'Failed', severity: 'error'};
    }
};

// ProbeResultLine is the inline verdict for one capability probe — an outlined
// Alert (mirroring probe's StatusBar) with the capability, the one-glance
// outcome, and a collapsible Raw JSON. Dismissible so the row stays clean.
const ProbeResultLine: React.FC<{result: ChatProbeResult; onDismiss: () => void}> = ({result, onDismiss}) => {
    const {t} = useTranslation();
    const [showRaw, setShowRaw] = useState(false);
    const v = verdict(result);
    return (
        <Alert
            severity={v.severity}
            variant="outlined"
            icon={false}
            sx={{py: 0.5, borderRadius: 1, '& .MuiAlert-message': {width: '100%'}}}
        >
            <Box sx={{display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap'}}>
                <Chip label={result.capability} size="small" sx={{textTransform: 'capitalize', fontWeight: 600}} />
                <Typography variant="body2" sx={{fontWeight: 600, color: v.severity === 'success' ? 'success.main' : v.severity === 'error' ? 'error.main' : 'warning.main'}}>
                    {v.label}
                </Typography>
                {result.reason && (
                    <Typography variant="caption" sx={{color: 'text.secondary'}}>{result.reason}</Typography>
                )}
                <Box sx={{flexGrow: 1}} />
                <Tooltip title={t('notify.probe.showRaw', {defaultValue: 'Show raw payload'})}>
                    <IconButton size="small" onClick={() => setShowRaw((s) => !s)}>
                        <CodeIcon fontSize="small" />
                    </IconButton>
                </Tooltip>
                <Tooltip title={t('common.dismiss', {defaultValue: 'Dismiss'})}>
                    <IconButton size="small" onClick={onDismiss}>
                        <CloseIcon fontSize="small" />
                    </IconButton>
                </Tooltip>
            </Box>
            <Collapse in={showRaw}>
                <Box sx={{mt: 1, p: 1, bgcolor: 'action.hover', borderRadius: 1, fontFamily: fontMono, fontSize: '0.75rem', whiteSpace: 'pre-wrap', wordBreak: 'break-all'}}>
                    {JSON.stringify(result.raw ?? result, null, 2)}
                </Box>
            </Collapse>
        </Alert>
    );
};



const BotNotifyGroup: React.FC<BotNotifyGroupProps> = ({bot, onToggle, isToggling}) => {
    const {t} = useTranslation();
    const capabilityOn = capabilityEnabled(bot, 'notify');
    const enabled = (bot.enabled ?? true) && capabilityOn;

    const [chats, setChats] = useState<BotChat[]>([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [testChatID, setTestChatID] = useState<string | null>(null);
    const [showDisabled, setShowDisabled] = useState(false);
    const [busyChat, setBusyChat] = useState<string | null>(null); // chat_id with an in-flight mutation
    const [deleteTarget, setDeleteTarget] = useState<string | null>(null); // chat_id pending confirm

    const loadChats = useCallback(async () => {
        if (!bot.uuid) return;
        setLoading(true);
        setError(null);
        // Always fetch with include_disabled — the visible set is filtered
        // client-side, so the "Show disabled" toggle and post-mutation
        // refreshes don't need extra round-trips.
        const result = await api.listBotChats(bot.uuid);
        setLoading(false);
        if (result.error) {
            setError(result.error);
        } else {
            setChats(result.chats ?? []);
        }
    }, [bot.uuid]);

    // Eager-load only when the bot is enabled (a stopped bot has no reachable
    // chats). Re-fetch on enable transitions so toggling on surfaces fresh chats.
    useEffect(() => {
        if (enabled) loadChats();
        else {
            setChats([]);
            setError(null);
            // A fetch may still be in flight from before the toggle — clear
            // loading so the graph (with its disabled body) renders instead of
            // a spinner that only resolves when the stale response lands.
            setLoading(false);
        }
    }, [enabled, loadChats]);

    const handleCopy = useCallback(async (targetID: string) => {
        try {
            await navigator.clipboard.writeText(targetID);
            notify.success(t('notify.chat.copied', {defaultValue: 'Target UUID copied'}));
        } catch {
            notify.error(t('notify.chat.copyFailed', {defaultValue: 'Copy failed — check clipboard permissions'}));
        }
    }, [t]);

    const openTest = useCallback((chatID: string) => setTestChatID(chatID), []);
    const closeTest = useCallback(() => setTestChatID(null), []);

    // Chat lifecycle actions — disable (inbound blocklist; the chat also drops
    // out of notify/interact) and hard delete (record removed; the chat
    // re-registers fresh if it messages the bot again).
    const handleToggleDisabled = useCallback(async (chat: BotChat) => {
        if (!bot.uuid) return;
        const target = !chat.blocked;
        setBusyChat(chat.chat_id);
        const result = await api.setBotDirectChatBlocked(bot.uuid, chat.id, target);
        setBusyChat(null);
        if (result.error) {
            notify.error(result.error);
            return;
        }
        notify.success(target
            ? t('notify.chat.disabled', {defaultValue: 'Chat disabled — its messages are now dropped'})
            : t('notify.chat.enabled', {defaultValue: 'Chat re-enabled'}));
        setChats(prev => prev.map(c => c.id === chat.id ? {...c, blocked: target} : c));
    }, [bot.uuid, t]);

    const handleDelete = useCallback(async (chatID: string) => {
        if (!bot.uuid) return;
        const chat = chats.find((candidate) => candidate.chat_id === chatID);
        if (!chat) return;
        setBusyChat(chatID);
        const result = await api.deleteBotDirectChat(bot.uuid, chat.id);
        setBusyChat(null);
        setDeleteTarget(null);
        if (result.error) {
            notify.error(result.error);
            return;
        }
        notify.success(t('notify.chat.deleted', {defaultValue: 'Chat deleted'}));
        setChats(prev => prev.filter(c => c.chat_id !== chatID));
    }, [bot.uuid, chats, t]);

    // The capability probe runner — owns firing notify/confirm against a chat
    // and the per-(chat,capability) results. Lives at the group level so a
    // result persists across re-renders of the chat list.
    const probe = useChatProbe();
    const handleProbe = useCallback((chat: BotChat, capability: ChatCapability) => {
        if (!bot.uuid) return;
        void probe.run(bot.uuid, chat.chat_id, chat.id, capability);
    }, [bot.uuid, probe]);

    // Disabled chats are hidden by default; the footer toggle reveals them
    // (dimmed) so they can be re-enabled or deleted.
    const activeChats = chats.filter(c => !c.blocked);
    const disabledCount = chats.length - activeChats.length;
    const visibleChats = showDisabled ? chats : activeChats;

    return (
        <Box
            sx={(theme) => ({
                position: 'relative',
                border: '1px solid',
                borderColor: 'divider',
                borderRadius: 1.5,
                overflow: 'hidden',
                // Bot off = the same diagonal-hatch "deliberately not running"
                // affordance the bot cards use (nodes/styles.tsx).
                // Pointer-transparent, so the enabled switch stays usable
                // through the overlay.
                ...(!enabled && getInactiveHatchSx(theme)),
            })}
        >
            {/* Header: name + platform + enabled switch (the on/off for driving
                this bot) + chat count. The switch is the bot's existing enabled
                flag — surfaced here because "can I use this bot to notify?" is
                exactly the question this page answers. */}
            <Box
                sx={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 1.5,
                    px: 2,
                    py: 1.25,
                    bgcolor: 'action.hover',
                    flexWrap: 'wrap',
                }}
            >
                {/* Fixed-width name column so every group's name chip aligns
                    across rows — name length varies, but the column shouldn't. */}
                <Tooltip title={bot.name || bot.platform}>
                    <Typography noWrap variant="body2" sx={{fontWeight: 600, flexShrink: 0, width: {xs: 96, sm: 150}}}>
                        {bot.name || bot.platform}
                    </Typography>
                </Tooltip>
                <Chip label={bot.platform} size="small" />
                <Box sx={{flexGrow: 1}} />
                {enabled && (
                    <Typography variant="body2" sx={{color: 'text.secondary'}}>
                        {activeChats.length > 0
                            ? t('notify.group.chatCount', {defaultValue: '{{count}} reachable chat(s)', count: activeChats.length})
                            : t('notify.group.noChats', {defaultValue: 'No reachable chats'})}
                    </Typography>
                )}
                {enabled && (
                    // Manual refresh: a chat only registers after the bot
                    // actually receives a message on its channel, so the first
                    // view is expected to be stale until the operator re-pulls.
                    <Tooltip title={t('notify.group.refresh', {defaultValue: 'Refresh reachable chats'})}>
                        <IconButton
                            size="small"
                            onClick={loadChats}
                            disabled={loading || isToggling}
                            aria-label={t('notify.group.refresh', {defaultValue: 'Refresh reachable chats'})}
                        >
                            {loading ? <CircularProgress size={16}/> : <RefreshIcon fontSize="small"/>}
                        </IconButton>
                    </Tooltip>
                )}
                <Tooltip title={enabled
                    ? t('notify.group.disableHint', {defaultValue: 'Disable Notify for this bot'})
                    : t('notify.group.enableHint', {defaultValue: 'Enable Notify. The bot starts automatically if needed.'})}>
                    {/* Present one operational state. The backend reconciles
                        the capability and Bot lifecycle as one action. */}
                    <Stack
                        direction="row"
                        spacing={0.75}
                        sx={{alignItems: 'center', cursor: isToggling ? 'wait' : 'pointer'}}
                    >
                        <Switch
                            size="small"
                            color="success"
                            checked={enabled}
                            disabled={isToggling}
                            onChange={(_, checked) => onToggle(bot.uuid!, checked)}
                        />
                        {isToggling ? (
                            <CircularProgress size={14} />
                        ) : (
                            <Typography variant="body2" sx={{color: enabled ? 'success.main' : 'text.secondary', fontWeight: 600}}>
                                {enabled ? t('common.on', {defaultValue: 'On'}) : t('common.off', {defaultValue: 'Off'})}
                            </Typography>
                        )}
                    </Stack>
                </Tooltip>
            </Box>

            {/* Body: the notify routing graph — API entry → bot channel →
                chat leaves. Same visual language as RemoteControlGraph
                (nodes + arrows + fork border), opposite direction: remote
                control is chat-driven (chat → bot → agent), notify is
                API-driven (API → bot → chat). State is topology: bot off dims
                the whole chain, a disabled chat dims only its leaf. */}
            <Box sx={{px: {xs: 1, sm: 2}, py: 1.5}}>
                {loading ? (
                    <Box sx={{display: 'flex', justifyContent: 'center', py: 2}}>
                        <CircularProgress size={20} />
                    </Box>
                ) : error ? (
                    <Typography variant="body2" sx={{color: 'error.main', py: 1}}>{error}</Typography>
                ) : (
                    <Box sx={graphRowStyles}>
                        {/* Source: the authenticated API surface — the concrete
                            path (real uuid, /api/v1 prefix) so the tooltip is a
                            copyable curl target (ux-principles #5/#11). */}
                        <NodeContainer>
                            <ApiEntryNode path={`/api/v1/bots/${bot.uuid}/notify`} active={enabled} />
                        </NodeContainer>

                        <ArrowNode direction="forward" />

                        {/* The bot channel the notify API drives — unlike the
                            remote graph (whose entry is the platform traffic
                            comes FROM), notify targets this specific bot, so
                            the node carries the bot identity. */}
                        <NodeContainer>
                            <ImBotNode imbot={bot} active={enabled} />
                        </NodeContainer>

                        <ArrowNode direction="forward" />

                        {/* Fork: one branch per reachable chat (mirrors the @tb/@cc
                            fork in RemoteControlGraph). */}
                        {visibleChats.length === 0 ? (
                            <Typography variant="body2" sx={{color: 'text.disabled', py: 1, minWidth: 220}}>
                                {!enabled
                                    ? t('notify.group.disabledBody', {defaultValue: 'Bot is off — enable it to see and send to its reachable chats.'})
                                    : isPairingRequired(bot)
                                        ? t('notify.group.emptyPairFirst', {defaultValue: 'No chats yet. Pair this bot, then send it a message on {{platform}} — its Chat ID appears here.', platform: bot.platform || 'its platform'})
                                        : t('notify.group.empty', {defaultValue: 'No chats yet. Send any message to this bot on {{platform}} and its Chat ID appears here.', platform: bot.platform || 'its platform'})}
                            </Typography>
                        ) : (
                            <Box
                                sx={{
                                    display: 'flex',
                                    flexDirection: 'column',
                                    gap: 2,
                                    borderLeft: '2px solid',
                                    borderColor: 'divider',
                                    pl: 2,
                                    py: 0.5,
                                }}
                            >
                                {visibleChats.map((chat) => {
                                    const running = (cap: ChatCapability) => probe.isRunning(chat.chat_id, cap);
                                    const anyRunning = running('notify') || running('confirm');
                                    // One lookup per capability per render — reused by the
                                    // button colors and the verdict lines below.
                                    const results = (['notify', 'confirm'] as const)
                                        .map((cap) => ({cap, result: probe.getResult(chat.chat_id, cap)}))
                                        .filter((r): r is {cap: ChatCapability; result: ChatProbeResult} => Boolean(r.result));
                                    const resultFor = (cap: ChatCapability) => results.find((r) => r.cap === cap)?.result;
                                    // The probe bench is only usable when the chain is live:
                                    // bot on and chat not blocklisted (a disabled chat 404s).
                                    const benchUsable = enabled && !chat.blocked && chat.can_notify;
                                    return (
                                        <Box key={chat.chat_id} sx={{display: 'flex', alignItems: 'flex-start', gap: 1.5}}>
                                            {/* The chat leaf. */}
                                            <NodeContainer>
                                                <ChatNode
                                                    chatID={chat.chat_id}
                                                    targetID={chat.id}
                                                    isPaired={chat.is_paired}
                                                    projectPath={chat.project_path}
                                                    updatedAt={chat.updated_at}
                                                    active={enabled}
                                                    blocked={chat.blocked}
                                                />
                                            </NodeContainer>

                                            {/* Beside the node, vertically centered: one action
                                                row — probe bench left, lifecycle icons pushed
                                                right — with verdicts underneath. One row, two
                                                zones: "use the chat" vs "manage the chat". */}
                                            <Box sx={{display: 'flex', flexDirection: 'column', gap: 0.75, minWidth: 0, flex: 1, justifyContent: 'center', alignSelf: 'stretch'}}>
                                                <Box sx={{display: 'flex', alignItems: 'center', gap: 0.75, flexWrap: 'wrap'}}>
                                                    {/* Probe bench — hidden for a disabled chat:
                                                        the backend 404s pushes to it, so the
                                                        buttons would only manufacture failures.
                                                        Gated (not-yet-wired) capabilities are
                                                        skipped, not rendered dead. */}
                                                    {benchUsable && (<>
                                                        {CHAT_CAPABILITIES.filter((cap) => !cap.gated).map((cap) => {
                                                            const capability = cap.capability as ChatCapability;
                                                            const isRunning = running(capability);
                                                            const result = resultFor(capability);
                                                            const v = result ? verdict(result) : null;
                                                            return (
                                                                <Tooltip key={cap.capability} title={cap.hint}>
                                                                    <span>
                                                                        <Button
                                                                            size="small"
                                                                            variant="outlined"
                                                                            color={v ? (v.severity === 'info' ? 'primary' : v.severity) : 'primary'}
                                                                            disabled={isRunning || anyRunning}
                                                                            onClick={() => handleProbe(chat, capability)}
                                                                            startIcon={isRunning ? <CircularProgress size={14} color="inherit" /> : cap.icon}
                                                                            sx={{textTransform: 'none'}}
                                                                        >
                                                                            {cap.label}
                                                                        </Button>
                                                                    </span>
                                                                </Tooltip>
                                                            );
                                                        })}
                                                        {/* Custom → free-form editor. Same outlined
                                                            variant as the probe buttons — it sits in
                                                            the same bench and acts on the same chat. */}
                                                        <Tooltip title={t('notify.group.customHint', {defaultValue: 'Compose a custom message (free-form)'})}>
                                                            <Button
                                                                size="small"
                                                                variant="outlined"
                                                                color="primary"
                                                                startIcon={<CustomIcon fontSize="small" />}
                                                                onClick={() => openTest(chat.chat_id)}
                                                                sx={{textTransform: 'none'}}
                                                            >
                                                                {t('notify.group.custom', {defaultValue: 'Custom'})}
                                                            </Button>
                                                        </Tooltip>
                                                    </>)}

                                                    <Box sx={{flexGrow: 1}} />

                                                    {/* Lifecycle zone: copy · disable · delete. */}
                                                    <Tooltip title={t('notify.group.copyChatId', {defaultValue: 'Copy internal target UUID'})}>
                                                        <IconButton size="small" onClick={() => handleCopy(chat.id)}>
                                                            <CopyIcon fontSize="small" />
                                                        </IconButton>
                                                    </Tooltip>
                                                    <Tooltip title={chat.blocked
                                                        ? t('notify.group.enableChat', {defaultValue: 'Enable — accept its messages again'})
                                                        : t('notify.group.disableChat', {defaultValue: 'Disable — silently drop its messages'})}>
                                                        <span>
                                                            <IconButton
                                                                size="small"
                                                                color={chat.blocked ? 'default' : 'warning'}
                                                                disabled={busyChat === chat.chat_id}
                                                                onClick={() => handleToggleDisabled(chat)}
                                                                aria-label={chat.blocked
                                                                    ? t('notify.group.enableChat', {defaultValue: 'Enable chat'})
                                                                    : t('notify.group.disableChat', {defaultValue: 'Disable chat'})}
                                                            >
                                                                <BlockIcon fontSize="small" />
                                                            </IconButton>
                                                        </span>
                                                    </Tooltip>
                                                    <Tooltip title={t('notify.group.deleteChat', {defaultValue: 'Delete this chat record'})}>
                                                        <span>
                                                            <IconButton
                                                                size="small"
                                                                color="error"
                                                                disabled={busyChat === chat.chat_id}
                                                                onClick={() => setDeleteTarget(chat.chat_id)}
                                                                aria-label={t('notify.group.deleteChat', {defaultValue: 'Delete chat'})}
                                                            >
                                                                <DeleteIcon fontSize="small" />
                                                            </IconButton>
                                                        </span>
                                                    </Tooltip>
                                                </Box>

                                                {/* Inline probe results (one per active capability that has run). */}
                                                {results.length > 0 && (
                                                    <Stack spacing={0.75}>
                                                        {results.map(({cap, result}) => (
                                                            <ProbeResultLine key={cap} result={result} onDismiss={() => probe.clear(chat.chat_id, cap)} />
                                                        ))}
                                                    </Stack>
                                                )}
                                            </Box>
                                        </Box>
                                    );
                                })}
                            </Box>
                        )}
                    </Box>
                )}
                {!loading && !error && enabled && disabledCount > 0 && (
                    <Box sx={{mt: 1, textAlign: 'right'}}>
                        <Link
                            component="button"
                            variant="caption"
                            underline="hover"
                            onClick={() => setShowDisabled(v => !v)}
                            sx={{color: 'text.secondary'}}
                        >
                            {showDisabled
                                ? t('notify.group.hideDisabled', {defaultValue: 'Hide disabled'})
                                : t('notify.group.showDisabled', {defaultValue: 'Show disabled ({{count}})', count: disabledCount})}
                        </Link>
                    </Box>
                )}
            </Box>

            {/* Delete confirm — hard delete is destructive-but-recoverable
                (natural re-register), so the dialog states exactly what goes
                and points to Disable as the blocking alternative. */}
            <ConfirmDialog
                open={deleteTarget !== null}
                title={t('notify.group.deleteChatTitle', {defaultValue: 'Delete this chat?'})}
                description={
                    <>
                        <Typography variant="body2" sx={{fontFamily: fontMono, mb: 1}}>{deleteTarget}</Typography>
                        <Typography variant="body2">
                            {t('notify.group.deleteChatBody', {
                                defaultValue: 'Its pairing, whitelist, and project binding are removed. If it messages the bot again it re-registers as a brand-new chat (re-pairing required when pairing is enforced). Session history is untouched. To block it instead, use Disable.',
                            })}
                        </Typography>
                    </>
                }
                confirmLabel={t('common.delete', {defaultValue: 'Delete'})}
                confirmColor="error"
                loading={busyChat !== null && busyChat === deleteTarget}
                onClose={() => setDeleteTarget(null)}
                onConfirm={() => deleteTarget && handleDelete(deleteTarget)}
            />

            <NotifyTestDialog
                open={testChatID !== null}
                botUUID={bot.uuid!}
                botName={bot.name || bot.platform}
                chats={activeChats}
                initialChatID={testChatID ?? undefined}
                onClose={closeTest}
            />
        </Box>
    );
};

export default BotNotifyGroup;
