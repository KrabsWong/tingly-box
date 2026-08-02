import {Box, Chip, Divider, Typography} from '@mui/material';
import {NODE_LAYER_STYLES, StyledBotGraphNode} from './styles';
import NodeTooltip from './NodeTooltip';
import {fontMono} from '@/theme/fonts';

// ApiEntryNode is the source of the notify routing graph: the authenticated
// HTTP surface (POST /api/v1/bots/:bot/notify|interact) that drives a bot's
// channel. Same dimensions as PlatformNode/ChatNode so the graph rows align.

export interface ApiEntryNodeProps {
    /** Path shown on the node, e.g. "/api/v1/bots/:bot/notify". */
    path: string;
    active?: boolean;
    /** Optional click-through (e.g. open the API guide / curl example). */
    onClick?: () => void;
}

const ApiEntryNode: React.FC<ApiEntryNodeProps> = ({path, active = true, onClick}) => {
    return (
        <StyledBotGraphNode active={active} clickable={!!onClick} onClick={onClick}>
            <Box sx={NODE_LAYER_STYLES.topLayer}>
                <NodeTooltip title={<>POST {path}<br/>Authenticated with the operator user token.</>} placement="top">
                    <Typography
                        variant="body2"
                        noWrap
                        sx={{
                            ...NODE_LAYER_STYLES.typography,
                            fontFamily: fontMono,
                            fontSize: '0.72rem',
                            maxWidth: 190,
                            color: 'text.primary',
                        }}
                    >
                        {path}
                    </Typography>
                </NodeTooltip>
            </Box>
            <Divider sx={NODE_LAYER_STYLES.divider}/>
            <Box sx={NODE_LAYER_STYLES.bottomLayer}>
                <Chip
                    label="API"
                    size="small"
                    color={active ? 'primary' : 'default'}
                    sx={{height: 24, fontSize: '0.7rem', fontWeight: 500}}
                />
            </Box>
        </StyledBotGraphNode>
    );
};

export default ApiEntryNode;
