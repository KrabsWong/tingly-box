import { Box, Typography, Divider, Chip } from '@mui/material';
import { Warning as WarningIcon } from '@/components/icons';
import { NODE_LAYER_STYLES, StyledBotGraphNode } from './styles';
import NodeTooltip from './NodeTooltip';
import { useCallback } from 'react';

interface BotModelNodeProps {
    provider?: string;
    providerName?: string;  // Display name of the provider
    model?: string;
    active?: boolean;
    onClick?: () => void;
}

const BotModelNode: React.FC<BotModelNodeProps> = ({
    provider,
    providerName,
    model,
    active = true,
    onClick,
}) => {
    const clickable = !!onClick;
    const hasConfig = !!(provider && model);

    const handleClick = useCallback((event: React.MouseEvent) => {
        event.stopPropagation();
        if (onClick) onClick();
    }, [onClick]);

    return (
        <StyledBotGraphNode active={active} clickable={clickable} warn={!hasConfig} onClick={handleClick}>
            {/* Top Layer - Provider name and model display (same as ProviderNode) */}
            <Box sx={NODE_LAYER_STYLES.topLayer}>
                <NodeTooltip title={
                    hasConfig
                        ? <>Provider: {providerName || provider}<br/>Model: {model}</>
                        : 'Click to configure bot model'
                } placement="top">
                    <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 0.5 }}>
                        {/* Warning icon when model not configured - inline with text */}
                        {active && !hasConfig && (
                            <WarningIcon
                                sx={{
                                    fontSize: '1rem',
                                    color: 'warning.main',
                                }}
                            />
                        )}

                        <Typography
                            variant="body2"
                            noWrap
                            sx={{
                                color: "text.primary",
                                ...NODE_LAYER_STYLES.typography,
                                fontStyle: !provider ? 'italic' : 'normal',
                                width: '100px',
                                textAlign: 'center'
                            }}>
                            {providerName || provider || 'select model'}
                        </Typography>

                        {provider && (
                            <Divider orientation="vertical" flexItem sx={{ mx: 0.5 }} />
                        )}

                        {provider && (
                            <Typography
                                variant="body2"
                                noWrap
                                sx={{
                                    color: "text.primary",
                                    ...NODE_LAYER_STYLES.typography,
                                    fontStyle: !model ? 'italic' : 'normal',
                                    width: '70px',
                                    textAlign: 'center'
                                }}>
                                {model || 'select model'}
                            </Typography>
                        )}
                    </Box>
                </NodeTooltip>
            </Box>
            <Divider sx={NODE_LAYER_STYLES.divider} />
            {/* Bottom Layer - Chip showing bot model */}
            <Box sx={NODE_LAYER_STYLES.bottomLayer}>
                <Chip
                    label="Model"
                    size="small"
                    color={hasConfig ? 'warning' : 'default'}
                    sx={{ height: 24, fontSize: '0.7rem', fontWeight: 500 }}
                />
            </Box>
        </StyledBotGraphNode>
    );
};

export default BotModelNode;
