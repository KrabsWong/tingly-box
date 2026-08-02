import { Box, Typography, Dialog, DialogTitle, DialogContent, DialogActions, Button, TextField, Divider } from '@mui/material';
import { NODE_LAYER_STYLES, StyledBotGraphNode } from './styles';
import { useState } from 'react';

interface CWDNodeProps {
    onPathChange?: (path: string) => void;
    disabled?: boolean;
    currentPath?: string;
}

// Check if window.showDirectoryPicker is available
const hasDirectoryPicker = () => {
    return typeof window !== 'undefined' && 'showDirectoryPicker' in window;
};

const CWDNode: React.FC<CWDNodeProps> = ({ onPathChange, disabled = false, currentPath = '' }) => {
    const [dialogOpen, setDialogOpen] = useState(false);
    const [pathInput, setPathInput] = useState(currentPath);

    const handleClick = () => {
        if (disabled) return;

        // Try browser directory picker first, fallback to manual input
        if (hasDirectoryPicker()) {
            handleSelectFolder();
        } else {
            handleOpenDialog();
        }
    };

    const handleSelectFolder = async () => {
        try {
            // @ts-ignore - window.showDirectoryPicker is not in standard TypeScript types
            const handle = await window.showDirectoryPicker({ mode: 'read' });
            if (handle) onPathChange?.(handle.name);
        } catch (err) {
            console.error('Failed to select directory:', err);
            // If user didn't cancel, open manual input dialog
            if ((err as Error).name !== 'AbortError') {
                handleOpenDialog();
            }
        }
    };

    const handleOpenDialog = () => {
        setDialogOpen(true);
        setPathInput(currentPath);
    };

    const handleSavePath = () => {
        onPathChange?.(pathInput);
        setDialogOpen(false);
    };

    const displayPath = currentPath
        ? currentPath.split('/').pop() || currentPath.split('\\').pop() || currentPath
        : 'Not set';

    return (
        <>
            <StyledBotGraphNode active={!disabled} clickable={!disabled} onClick={handleClick}>
                <Box sx={NODE_LAYER_STYLES.topLayer}>
                    <Typography variant="body2" sx={{
                        fontWeight: 600,
                        fontSize: '0.9rem',
                        color: disabled ? 'text.disabled' : 'text.primary',
                        maxWidth: 200,
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        whiteSpace: 'nowrap',
                    }}>
                        {displayPath}
                    </Typography>
                </Box>

                <Divider sx={NODE_LAYER_STYLES.divider} />

                <Box sx={NODE_LAYER_STYLES.bottomLayer}>
                    <Typography variant="caption" sx={{
                        fontWeight: 600,
                        fontSize: '0.7rem',
                        color: 'text.secondary',
                        textTransform: 'uppercase',
                    }}>Default Path</Typography>
                </Box>
            </StyledBotGraphNode>
            <Dialog open={dialogOpen} onClose={() => setDialogOpen(false)} maxWidth="sm" fullWidth>
                <DialogTitle>Set Working Directory</DialogTitle>
                <DialogContent>
                    <Box sx={{ mt: 2 }}>
                        <TextField
                            autoFocus
                            fullWidth
                            label="Working Directory Path"
                            value={pathInput}
                            onChange={(e) => setPathInput(e.target.value)}
                            placeholder="/path/to/directory or C:\\path\\to\\directory"
                            size="small"
                            helperText="Enter the absolute path to the working directory"
                        />
                        {currentPath && (
                            <Typography
                                variant="caption"
                                sx={{
                                    color: "text.secondary",
                                    mt: 1,
                                    display: 'block'
                                }}>
                                Current: {currentPath}
                            </Typography>
                        )}
                    </Box>
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setDialogOpen(false)}>Cancel</Button>
                    <Button onClick={handleSavePath} variant="contained" disabled={!pathInput.trim()}>Save</Button>
                </DialogActions>
            </Dialog>
        </>
    );
};

export default CWDNode;
