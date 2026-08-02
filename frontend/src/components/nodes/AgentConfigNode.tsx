import type { AgentConfig } from '@/types/remoteGraph';
import { CheckCircle as CheckCircleIcon, Settings as SettingsIcon } from '@/components/icons';
import { Box, Divider, styled } from '@mui/material';
import {
    AGENT_CONFIG_NODE_STYLES,
    NODE_LAYER_STYLES,
    getRouteGraphBorderColor,
    graphNodeBaseHoverStyles,
    graphNodeHoverStyles,
} from './styles';

// Smaller supplementary node — keeps its own 180×60 footprint but speaks the
// route-graph visual language (neutral alpha border, no static shadow, hover
// emphasis ring).
const StyledConfigNode = styled(Box)(({ theme }) => ({
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    padding: AGENT_CONFIG_NODE_STYLES.padding,
    borderRadius: theme.shape.borderRadius,
    border: '1px solid',
    borderColor: getRouteGraphBorderColor(theme),
    backgroundColor: theme.palette.background.paper,
    textAlign: 'center',
    width: AGENT_CONFIG_NODE_STYLES.width,
    height: AGENT_CONFIG_NODE_STYLES.height,
    transition: 'border-color 0.16s ease, background-color 0.16s ease, box-shadow 0.18s ease, transform 0.18s ease',
    position: 'relative',
    cursor: 'pointer',
    ...graphNodeBaseHoverStyles,
    '&:hover': graphNodeHoverStyles(theme),
}));

interface AgentConfigNodeProps {
    agentConfig: AgentConfig;
    configured?: boolean;
    onClick?: () => void;
}

const AgentConfigNode: React.FC<AgentConfigNodeProps> = ({ agentConfig, configured = false, onClick }) => {
    return (
        <StyledConfigNode onClick={onClick}>
            <Box sx={NODE_LAYER_STYLES.topLayer}>
                <Box sx={{ position: 'relative' }}>
                    <SettingsIcon sx={{ fontSize: 24, color: configured ? 'primary.main' : 'text.disabled' }} />
                    {configured && (
                        <CheckCircleIcon sx={{
                            position: 'absolute',
                            bottom: -4,
                            right: -4,
                            fontSize: 12,
                            color: 'success.main',
                            backgroundColor: 'background.paper',
                            borderRadius: '50%',
                        }} />
                    )}
                </Box>
            </Box>

            <Divider sx={NODE_LAYER_STYLES.divider} />

            <Box sx={NODE_LAYER_STYLES.bottomLayer}>
                <Box
                    component="span"
                    sx={{
                        fontSize: '0.7rem',
                        fontWeight: 600,
                        color: configured ? 'text.primary' : 'text.secondary',
                    }}
                >
                    {configured ? 'Configured' : 'Configure'}
                </Box>
            </Box>
        </StyledConfigNode>
    );
};

export default AgentConfigNode;
