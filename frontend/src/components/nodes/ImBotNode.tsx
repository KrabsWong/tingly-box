import {Box, Chip, styled} from '@mui/material';
import {
    SMALL_NODE_STYLES,
    BOT_NODE_STYLES,
    NODE_LAYER_STYLES,
    getRouteGraphBorderColor,
    graphNodeBaseHoverStyles,
    graphNodeHoverStyles,
} from './styles';
import NodeTooltip from './NodeTooltip';
import type {BotSettings} from '@/types/bot';
import {useTranslation} from 'react-i18next';
import {platformDisplayName} from '@/constants/platformGuides';

interface ImBotNodeProps {
    imbot: BotSettings;
    active?: boolean;
    onClick?: () => void;
}

// ImBotNode is the bot channel the notify API drives — sized to the same
// 100×76 footprint as PlatformNode so the two read as siblings in the notify
// graph, but text-based: the bot name (top) and the platform as a tag on the
// bottom row, matching the type-Chip the other nodes carry. Name + UUID
// repeat in the tooltip.
const StyledImBotNode = styled(Box, {
    shouldForwardProp: (prop) => prop !== 'active' && prop !== 'clickable',
})<{ active?: boolean; clickable?: boolean }>(({active = true, clickable = false, theme}) => ({
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    padding: SMALL_NODE_STYLES.padding,
    borderRadius: theme.shape.borderRadius,
    border: '1px solid',
    borderColor: getRouteGraphBorderColor(theme),
    backgroundColor: theme.palette.background.paper,
    width: SMALL_NODE_STYLES.width,
    height: BOT_NODE_STYLES.height, // row-aligned with the other notify-graph nodes
    transition: 'border-color 0.16s ease, background-color 0.16s ease, box-shadow 0.18s ease, transform 0.18s ease',
    position: 'relative',
    opacity: active ? 1 : 0.6,
    cursor: clickable ? 'pointer' : 'default',
    ...graphNodeBaseHoverStyles,
    ...(clickable && {'&:hover': graphNodeHoverStyles(theme)}),
}));

const ImBotNode: React.FC<ImBotNodeProps> = ({imbot, active = true, onClick}) => {
    const {t} = useTranslation();
    const clickable = !!onClick && active;
    const platformLabel = platformDisplayName(imbot.platform, t);
    const name = imbot.name || 'Bot';

    return (
        <NodeTooltip
            title={
                <>
                    <Box component="span" sx={{fontWeight: 600}}>
                        {imbot.name || platformLabel}
                    </Box>
                    {imbot.uuid && (
                        <>
                            <br/>
                            <Box component="span" sx={{fontSize: '0.7rem'}}>
                                {t('nodes.imBotUUID', {defaultValue: 'Bot UUID'})}: {imbot.uuid}
                            </Box>
                        </>
                    )}
                </>
            }
            placement="top"
        >
            <StyledImBotNode active={active} clickable={clickable} onClick={onClick}>
                {/* Top - bot name (the identity the notify API targets) */}
                <Box sx={NODE_LAYER_STYLES.topLayer}>
                    <Box
                        component="span"
                        sx={{
                            ...NODE_LAYER_STYLES.typography,
                            fontStyle: !imbot.name ? 'italic' : 'normal',
                            textAlign: 'center',
                            color: active ? 'text.primary' : 'text.disabled',
                            fontSize: '0.75rem',
                            lineHeight: 1.1,
                            overflow: 'hidden',
                            textOverflow: 'ellipsis',
                            whiteSpace: 'nowrap',
                            maxWidth: '100%',
                        }}
                    >
                        {name}
                    </Box>
                </Box>

                <Box sx={{width: '70%', borderTop: '1px solid', borderColor: 'divider', my: 0.25}}/>

                {/* Bottom - platform tag (the channel this bot runs on),
                    matching the type-Chip the sibling nodes carry on the
                    bottom row) */}
                <Box sx={NODE_LAYER_STYLES.bottomLayer}>
                    <Chip
                        label={platformLabel}
                        size="small"
                        sx={{height: 24, fontSize: '0.7rem', fontWeight: 500}}
                    />
                </Box>
            </StyledImBotNode>
        </NodeTooltip>
    );
};

export default ImBotNode;
