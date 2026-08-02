import {
    Delete as DeleteIcon,
    Edit as EditIcon,
    MoreVert as MoreVertIcon,
    RestartAlt as RestartIcon,
    Warning as WarningIcon,
} from '@/components/icons';
import {
    Box,
    Chip,
    Collapse,
    IconButton,
    ListItemIcon,
    ListItemText,
    Menu,
    MenuItem,
    Stack,
    Switch,
    Tooltip,
    Typography,
} from '@mui/material';
import {styled} from '@mui/material/styles';
import ConfirmDialog from '@/components/ConfirmDialog';
import type {BotGroupDetail, BotSettings, DirectChatDetail} from '@/types/bot';
import {capabilityEnabled} from '@/types/bot';
import type {Provider} from '@/types/provider';
import type {ProfileInfo} from '@/contexts/ProfileContext';
import {botCardSx, statusChipSx} from './botCardStyles';
import RemoteControlGraph from './RemoteControlGraph';
import BotAccessDialog from './BotAccessDialog';
import {api} from '@/services/api';
import {useCallback, useEffect, useState} from 'react';
import {useTranslation} from 'react-i18next';

const GraphContainer = styled(Box)(({theme}) => ({
    padding: '10px 16px',
    borderRadius: theme.shape.borderRadius,
    margin: '8px 16px 0',
    overflowX: 'auto',
}));

interface RemoteAgentBotCardProps {
    bot: BotSettings;
    providers: Provider[];
    onMountToggle: (mounted: boolean) => void;
    onModelClick: () => void;
    /** Configured Claude Code profiles (resolves the @cc profile node label). */
    ccProfiles?: ProfileInfo[];
    /** Opens the Claude Code profile picker for this bot. */
    onCCProfileClick?: () => void;
    /** Opens the shared BotConfigDialog in edit mode (bot resource fields). */
    onEdit: () => void;
    onRestart: () => void;
    onDelete: () => void;
    isToggling?: boolean;
    isRestarting?: boolean;
    onAccessChanged?: () => void;
}

// RemoteAgentBotCard is the PURPOSE card: one row per bot on the Remote page.
// The mount switch decides whether this bot drives Claude Code / SmartGuide
// from chat; below it live the agent behavior (model, chat lock, allowlist).
//
// While the Bots nav section is hidden (bot has a single purpose today), this
// card also hosts the bot RESOURCE operations so the Remote page is fully
// self-sufficient: edit (shared BotConfigDialog) and restart/delete (overflow
// menu). Access and pairing live in the Access work surface opened from the
// first graph node, keeping authorization under one source of truth.
const RemoteAgentBotCard: React.FC<RemoteAgentBotCardProps> = ({
    bot,
    providers,
    onMountToggle,
    onModelClick,
    ccProfiles,
    onCCProfileClick,
    onEdit,
    onRestart,
    onDelete,
    isToggling = false,
    isRestarting = false,
    onAccessChanged,
}) => {
    const {t} = useTranslation();
    const isMounted = capabilityEnabled(bot,'remote_control');
    const isEnabled = bot.enabled ?? true;
    const hasModel = Boolean(bot.smartguide_provider && bot.smartguide_model);

    const [menuAnchor, setMenuAnchor] = useState<null | HTMLElement>(null);
    const [deleteModalOpen, setDeleteModalOpen] = useState(false);
    const [accessDialogOpen, setAccessDialogOpen] = useState(false);
    const [accessLoading, setAccessLoading] = useState(false);
    const [accessError, setAccessError] = useState('');
    const [accessCounts, setAccessCounts] = useState({directChats: 0, groups: 0});

    const loadAccess = useCallback(async () => {
        if (!bot.uuid) return;
        setAccessLoading(true);
        setAccessError('');
        try {
            const [chatData, groupData] = await Promise.all([
                api.listBotDirectChats(bot.uuid),
                api.listBotGroups(bot.uuid),
            ]);
            const groupDetails = await Promise.all((groupData.groups || []).map((group: {id: string}) =>
                api.getBotGroup(bot.uuid!, group.id)));
            const directChats = (chatData.chats || []).filter((detail: DirectChatDetail) =>
                detail.permissions.some((permission) =>
                    permission.capability === 'remote_control' &&
                    permission.action === 'access' &&
                    permission.effect === 'allow'));
            const groups = groupDetails.filter((detail: BotGroupDetail) =>
                detail.capabilities.remote_control === 'allow');
            setAccessCounts({directChats: directChats.length, groups: groups.length});
        } catch (error) {
            setAccessError((error as Error).message);
        } finally {
            setAccessLoading(false);
        }
    }, [bot.uuid]);

    useEffect(() => { void loadAccess(); }, [loadAccess]);

    // Off (hatched) when this purpose isn't live: unmounted, or the bot
    // itself is disabled. Graph nodes remain editable through the overlay —
    // the hatch is purely a "this isn't running" affordance.
    const isActive = isMounted && isEnabled;

    return (
        <Box sx={botCardSx(isActive)}>
            {/* Header: bot identity (read-only here) + mount switch */}
            <Box sx={{
                display: 'flex', flexWrap: 'wrap', alignItems: 'center', justifyContent: 'space-between', gap: 1,
                px: 2, py: 1.5,
            }}>
                <Box sx={{display: 'flex', alignItems: 'center', gap: 1.5, minWidth: 0}}>
                    {/* Fixed-width name column — same rationale as BotCard:
                        keeps the platform chip aligned across rows. */}
                    <Typography noWrap variant="body2" sx={{fontWeight: 600, flexShrink: 0, width: {xs: 96, sm: 150}}}>
                        {bot.name || bot.platform}
                    </Typography>
                    <Chip label={bot.platform} size="small"/>
                    {isMounted && !hasModel && (
                        <Tooltip title={t('remoteControl.card.noModelConfigured', { defaultValue: 'No model configured - click to select a model' })}>
                            <WarningIcon sx={{fontSize: '1.1rem', color: 'warning.main'}}/>
                        </Tooltip>
                    )}
                </Box>
                <Box sx={{display: 'flex', alignItems: 'center', gap: 1.5}}>
                    <Stack direction="row" spacing={1} sx={{alignItems: 'center'}}>
                        <Tooltip title={isActive
                            ? t('remoteControl.card.remoteAgentOff', { defaultValue: 'Turn off Remote Control. The bot remains available to other capabilities.' })
                            : t('remoteControl.card.remoteAgentOn', { defaultValue: 'Turn on Remote Control. The bot starts automatically if needed.' })}>
                            <Switch checked={isActive} onChange={() => onMountToggle(!isActive)} size="small" color="primary" disabled={isToggling}/>
                        </Tooltip>
                        <Chip
                            label={isActive ? t('common.on', { defaultValue: 'On' }) : t('common.off', { defaultValue: 'Off' })}
                            size="small"
                            color={isActive ? 'success' : 'default'}
                            variant={isActive ? 'filled' : 'outlined'}
                            sx={statusChipSx}
                        />
                    </Stack>
                    <Stack direction="row" spacing={0.5} sx={{alignItems: 'center'}}>
                        <Tooltip title={t('remoteControl.card.edit', { defaultValue: 'Edit' })}>
                            <IconButton size="small" color="primary" onClick={onEdit} disabled={isToggling || isRestarting}>
                                <EditIcon fontSize="small"/>
                            </IconButton>
                        </Tooltip>
                        <IconButton size="small" onClick={(e) => setMenuAnchor(e.currentTarget)} disabled={isToggling || isRestarting}>
                            <MoreVertIcon fontSize="small"/>
                        </IconButton>
                        <Menu
                            anchorEl={menuAnchor}
                            open={Boolean(menuAnchor)}
                            onClose={() => setMenuAnchor(null)}
                        >
                            <MenuItem
                                onClick={() => { setMenuAnchor(null); onRestart(); }}
                                disabled={!isEnabled || isRestarting}
                            >
                                <ListItemIcon><RestartIcon fontSize="small"/></ListItemIcon>
                                <ListItemText>{t('remoteControl.card.restartBot', { defaultValue: 'Restart Bot' })}</ListItemText>
                            </MenuItem>
                            <MenuItem onClick={() => { setMenuAnchor(null); setDeleteModalOpen(true); }}>
                                <ListItemIcon><DeleteIcon fontSize="small" color="error"/></ListItemIcon>
                                <ListItemText sx={{color: 'error.main'}}>{t('remoteControl.card.delete', { defaultValue: 'Delete' })}</ListItemText>
                            </MenuItem>
                        </Menu>
                    </Stack>
                </Box>
            </Box>

            {/* Agent behavior — always fully shown. */}
            <Collapse in timeout="auto" unmountOnExit>
                <GraphContainer>
                    <RemoteControlGraph
                        imbot={bot}
                        providers={providers}
                        isBotEnabled={isActive}
                        readOnly={isToggling}
                        onModelClick={onModelClick}
                        ccProfiles={ccProfiles}
                        onCCProfileClick={onCCProfileClick}
                        directChatCount={accessCounts.directChats}
                        groupCount={accessCounts.groups}
                        accessLoading={accessLoading}
                        accessError={accessError}
                        onAccessClick={() => setAccessDialogOpen(true)}
                    />
                </GraphContainer>
            </Collapse>

            <ConfirmDialog
                open={deleteModalOpen}
                title={t('remoteControl.card.deleteTitle', { defaultValue: 'Delete Bot Configuration' })}
                description={t('remoteControl.card.deleteConfirm', { defaultValue: 'Are you sure you want to delete "{{name}}"? This action cannot be undone.', name: bot.name || bot.platform })}
                confirmLabel={t('remoteControl.card.delete', { defaultValue: 'Delete' })}
                confirmColor="error"
                onClose={() => setDeleteModalOpen(false)}
                onConfirm={() => { setDeleteModalOpen(false); onDelete(); }}
            />
            <BotAccessDialog
                open={accessDialogOpen}
                bot={bot}
                onClose={() => setAccessDialogOpen(false)}
                onChanged={() => {
                    void loadAccess();
                    onAccessChanged?.();
                }}
            />
        </Box>
    );
};

export default RemoteAgentBotCard;
