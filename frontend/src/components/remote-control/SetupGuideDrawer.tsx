import { Close } from '@/components/icons';
import {
    Box,
    Button,
    Divider,
    Drawer,
    IconButton,
    Stack,
    Typography,
} from '@mui/material';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

interface SetupGuideDrawerProps {
    open: boolean;
    platformName: string;
    children: ReactNode;
    onClose: () => void;
    onConnectBot: () => void;
}

const SetupGuideDrawer = ({
    open,
    platformName,
    children,
    onClose,
    onConnectBot,
}: SetupGuideDrawerProps) => {
    const { t } = useTranslation();

    const handleConnectBot = () => {
        onClose();
        onConnectBot();
    };

    return (
        <Drawer
            anchor="right"
            open={open}
            onClose={onClose}
            slotProps={{
                paper: {
                    sx: {
                        width: { xs: '100%', sm: 440 },
                        maxWidth: '100%',
                    },
                },
            }}
        >
            <Box
                sx={{
                    display: 'flex',
                    flexDirection: 'column',
                    height: '100%',
                }}
            >
                <Box
                    sx={{
                        display: 'flex',
                        alignItems: 'flex-start',
                        justifyContent: 'space-between',
                        gap: 2,
                        px: 3,
                        py: 2.5,
                    }}
                >
                    <Box sx={{ minWidth: 0 }}>
                        <Typography variant="h4" component="h2" sx={{ fontWeight: 600 }}>
                            {t('remoteControl.guide.title', {
                                defaultValue: '{{platform}} Setup Guide',
                                platform: platformName,
                            })}
                        </Typography>
                        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                            {t('remoteControl.guide.drawerHint', {
                                defaultValue: 'Connection steps, credentials, and examples',
                            })}
                        </Typography>
                    </Box>
                    <IconButton
                        aria-label={t('common.close', { defaultValue: 'Close' })}
                        onClick={onClose}
                        edge="end"
                    >
                        <Close />
                    </IconButton>
                </Box>
                <Divider />
                <Box sx={{ flex: 1, overflowY: 'auto', px: 3, py: 2.5 }}>
                    <Stack spacing={2}>{children}</Stack>
                </Box>
                <Divider />
                <Box sx={{ px: 3, py: 2 }}>
                    <Button fullWidth variant="contained" onClick={handleConnectBot}>
                        {t('remoteControl.bots.addBot', { defaultValue: 'Connect a bot' })}
                    </Button>
                </Box>
            </Box>
        </Drawer>
    );
};

export default SetupGuideDrawer;
