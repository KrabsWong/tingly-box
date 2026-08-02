import { Box, ButtonBase, Typography, alpha } from '@mui/material';
import type { ReactNode } from 'react';

export interface PlatformPickerItem {
    id: string;
    label: string;
    /**
     * Rendered with the tile's selected state. The brand logos default to
     * grayscale (createBrandIcon(..., true)) and carry that filter on the
     * <img> itself — a parent filter can't undo it — so the icon has to be
     * (re)rendered with `grayscale={!active}` to come back to full color
     * when its tile is selected. Hence a render fn, not a static node.
     */
    icon: ReactNode;
    activeIcon?: ReactNode;
    subtitle?: string;
}

interface PlatformPickerProps {
    items: PlatformPickerItem[];
    value: string;
    onChange: (id: string) => void;
}

// PlatformPicker is a compact horizontal context rail. Platform is a filter,
// not the page's primary task, so it must remain available without consuming
// an entire desktop viewport when every supported transport is present.
const PlatformPicker: React.FC<PlatformPickerProps> = ({ items, value, onChange }) => (
    <Box
        role="list"
        aria-label="Platforms"
        sx={{
            display: 'flex',
            gap: 1,
            mb: 2,
            pb: 0.5,
            overflowX: 'auto',
            overscrollBehaviorX: 'contain',
            scrollbarWidth: 'thin',
        }}
    >
        {items.map((item) => {
            const active = item.id === value;
            return (
                <ButtonBase
                    role="listitem"
                    key={item.id}
                    onClick={() => onChange(item.id)}
                    aria-current={active ? 'true' : undefined}
                    sx={{
                        flex: '0 0 auto',
                        minWidth: {xs: 138, sm: 156},
                        justifyContent: 'flex-start',
                        gap: 1,
                        px: 1.5,
                        py: 1,
                        borderRadius: 1.5,
                        border: '1px solid',
                        borderColor: active ? 'primary.main' : 'divider',
                        bgcolor: active
                            ? (theme) => alpha(theme.palette.primary.main, 0.08)
                            : 'background.paper',
                        transition: 'border-color 0.18s ease-out, background-color 0.18s ease-out',
                        '&:hover': {
                            borderColor: active ? 'primary.main' : 'text.disabled',
                            bgcolor: active
                                ? (theme) => alpha(theme.palette.primary.main, 0.08)
                                : 'action.hover',
                        },
                    }}
                >
                    <Box sx={{width: 24, height: 24, flexShrink: 0, display: 'grid', placeItems: 'center'}}>
                        {active ? (item.activeIcon || item.icon) : item.icon}
                    </Box>
                    <Box sx={{minWidth: 0, textAlign: 'left'}}>
                        <Typography variant="body2" noWrap sx={{fontWeight: 600, color: active ? 'primary.main' : 'text.primary'}}>
                            {item.label}
                        </Typography>
                        {item.subtitle && (
                            <Typography variant="caption" noWrap sx={{display: 'block', color: 'text.secondary'}}>
                                {item.subtitle}
                            </Typography>
                        )}
                    </Box>
                </ButtonBase>
            );
        })}
    </Box>
);

export default PlatformPicker;
