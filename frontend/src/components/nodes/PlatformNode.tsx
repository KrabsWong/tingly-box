import {Box} from '@mui/material';
import {useTranslation} from 'react-i18next';
import {styled} from '@mui/material/styles';
import {
    SMALL_NODE_STYLES,
    BOT_NODE_STYLES,
    getRouteGraphBorderColor,
    graphNodeBaseHoverStyles,
    graphNodeHoverStyles,
} from './styles';
import NodeTooltip from './NodeTooltip';
import {fontMono} from '@/theme/fonts';
import {PLATFORM_BRAND_ICONS, platformDisplayName} from '@/constants/platformGuides';

// PlatformNode is the entry of the remote-control graph and the channel hop
// of the notify graph: the IM platform traffic flows through. Which bot is
// involved is the card header's job (name + platform chip) — so the node is
// a compact icon-only marker (mid-size, like EntryNode), with the platform
// name in the tooltip. The brand icon is the identity; anything more would
// repeat the header.
const StyledPlatformNode = styled(Box, {
    shouldForwardProp: (prop) => prop !== 'active' && prop !== 'clickable',
})<{ active?: boolean; clickable?: boolean }>(({active = true, clickable = false, theme}) => ({
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    padding: SMALL_NODE_STYLES.padding,
    borderRadius: theme.shape.borderRadius,
    border: '1px solid',
    borderColor: getRouteGraphBorderColor(theme),
    backgroundColor: theme.palette.background.paper,
    width: SMALL_NODE_STYLES.width,
    height: BOT_NODE_STYLES.height, // row-aligned with the 220-wide card nodes
    transition: 'border-color 0.16s ease, background-color 0.16s ease, box-shadow 0.18s ease, transform 0.18s ease',
    position: 'relative',
    opacity: active ? 1 : 0.6,
    cursor: clickable ? 'pointer' : 'default',
    ...graphNodeBaseHoverStyles,
    ...(clickable && {'&:hover': graphNodeHoverStyles(theme)}),
}));

export interface PlatformNodeProps {
    /** Platform id, e.g. "telegram" (keys PLATFORM_BRAND_ICONS). */
    platform: string;
    /** Bot UUID behind this hop — surfaced in the tooltip for API lookups. */
    botUUID?: string;
    /** false when the bot's channel is off — the hop is dead. */
    active?: boolean;
    onClick?: () => void;
}

const PlatformNode: React.FC<PlatformNodeProps> = ({platform, botUUID, active = true, onClick}) => {
    const {t} = useTranslation();
    const BrandIcon = PLATFORM_BRAND_ICONS[platform];
    return (
        <NodeTooltip
            title={
                <>
                    {platformDisplayName(platform, t)} — {t('nodes.platformHint', {defaultValue: 'the IM platform this chain runs on'})}
                    {botUUID && (
                        <>
                            <br/>
                            <Box component="span" sx={{fontFamily: fontMono, fontSize: '0.7rem'}}>
                                {t('nodes.platformBotUUID', {defaultValue: 'Bot UUID'})}: {botUUID}
                            </Box>
                        </>
                    )}
                </>
            }
            placement="top"
        >
            <StyledPlatformNode active={active} clickable={!!onClick} onClick={onClick}>
                {BrandIcon
                    ? <BrandIcon size={32} grayscale={!active}/>
                    : <Box component="span" sx={{fontSize: '0.8rem', fontWeight: 600, color: 'text.secondary'}}>{platformDisplayName(platform, t)}</Box>}
            </StyledPlatformNode>
        </NodeTooltip>
    );
};

export default PlatformNode;
