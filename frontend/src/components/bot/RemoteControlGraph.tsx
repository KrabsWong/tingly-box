import { Box, useMediaQuery, useTheme } from '@mui/material';
import type {ReactNode} from 'react';
import type { BotSettings } from '@/types/bot.ts';
import { ccProfileIdFromDefaultAgent } from '@/types/bot.ts';
import type { Provider } from '@/types/provider.ts';
import type { ProfileInfo } from '@/contexts/ProfileContext';
import { AccessNode, ArrowNode, ImBotNode, NodeContainer, graphRowStyles } from '../nodes';
import BotModelNode from '../nodes/BotModelNode.tsx';
import AgentNode from '../nodes/AgentNode.tsx';
import AtNode from '../nodes/AtNode.tsx';
import CCProfileNode from '../nodes/CCProfileNode.tsx';

export interface RemoteControlGraphProps {
    imbot: BotSettings;
    providers: Provider[];
    isBotEnabled: boolean;
    readOnly?: boolean;
    onModelClick?: () => void;
    onBotClick?: () => void;
    onAccessClick?: () => void;
    directChatCount?: number;
    groupCount?: number;
    accessLoading?: boolean;
    accessError?: string;
    /** Configured Claude Code profiles (to resolve the selected profile name). */
    ccProfiles?: ProfileInfo[];
    /** Opens the Claude Code profile picker for this bot's @cc branch. */
    onCCProfileClick?: () => void;
}

const getProviderName = (providerUuid: string | undefined, providersData: Provider[]): string => {
    if (!providerUuid) return '';
    const provider = providersData.find(p => p.uuid === providerUuid);
    return provider?.name || '';
};

const RemoteControlGraph: React.FC<RemoteControlGraphProps> = ({
    imbot,
    providers,
    isBotEnabled,
    readOnly = false,
    onModelClick,
    onBotClick,
    ccProfiles,
    onCCProfileClick,
    onAccessClick,
    directChatCount = 0,
    groupCount = 0,
    accessLoading = false,
    accessError,
}) => {
    const theme = useTheme();
    const compact = useMediaQuery(theme.breakpoints.down('md'));
    const phone = useMediaQuery(theme.breakpoints.down('sm'));
    const providerName = getProviderName(imbot.smartguide_provider, providers);

    // Which Claude Code configuration serves @cc: '' = main claude_code
    // scenario, otherwise a profile ID from default_agent ("claude_code:<id>").
    const ccProfileId = ccProfileIdFromDefaultAgent(imbot.default_agent);
    const ccProfileName = ccProfiles?.find(p => p.id === ccProfileId)?.name;

    const accessNode = (
        <AccessNode
            directChats={directChatCount}
            groups={groupCount}
            active={isBotEnabled}
            loading={accessLoading}
            error={accessError}
            onClick={readOnly ? undefined : onAccessClick}
        />
    );
    const botNode = <ImBotNode imbot={imbot} active={isBotEnabled} onClick={readOnly ? undefined : onBotClick}/>;
    const tbNode = <AtNode type="tb"/>;
    const ccNode = <AtNode type="cc"/>;
    const smartGuideNode = <AgentNode agentType="smart-guide" active={isBotEnabled}/>;
    const claudeCodeNode = <AgentNode agentType="claude-code" active={isBotEnabled}/>;
    const modelNode = <BotModelNode provider={imbot.smartguide_provider} providerName={providerName} model={imbot.smartguide_model} active={isBotEnabled} onClick={readOnly ? undefined : onModelClick}/>;
    const profileNode = <CCProfileNode profileId={ccProfileId} profileName={ccProfileName} active={isBotEnabled} onClick={readOnly ? undefined : onCCProfileClick}/>;
    const smartGuideBranch = (<>
        <NodeContainer>{tbNode}</NodeContainer>
        <ArrowNode direction="forward" />
        <NodeContainer>{smartGuideNode}</NodeContainer>
        <ArrowNode direction="forward" />
        <NodeContainer>{modelNode}</NodeContainer>
    </>);
    const claudeCodeBranch = (<>
        <NodeContainer>{ccNode}</NodeContainer>
        <ArrowNode direction="forward" />
        <NodeContainer>{claudeCodeNode}</NodeContainer>
        <ArrowNode direction="forward" />
        <NodeContainer>{profileNode}</NodeContainer>
    </>);

    if (compact) {
        return (
            <Box>
                <Box sx={{display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 0.75}}>
                    {accessNode}
                    <ArrowNode direction="down" length={20}/>
                    {botNode}
                </Box>
                <Box sx={{display: 'grid', gap: 1.25, mt: 1.5}}>
                    {(phone ? [
                        [tbNode, smartGuideNode, modelNode],
                        [ccNode, claudeCodeNode, profileNode],
                    ] : [smartGuideBranch, claudeCodeBranch]).map((branch, index) => (
                        <Box
                            key={index === 0 ? 'tb' : 'cc'}
                            sx={{overflowX: phone ? 'visible' : 'auto', py: 1, px: 1.25, border: 1, borderColor: 'divider', borderRadius: 1.5, scrollbarWidth: 'thin'}}
                        >
                            <Box sx={{display: 'flex', flexDirection: phone ? 'column' : 'row', alignItems: 'center', gap: 1, width: phone ? '100%' : 'max-content', minWidth: '100%'}}>
                                {phone
                                    ? (branch as ReactNode[]).map((node, nodeIndex) => (
                                        <Box key={nodeIndex} sx={{display: 'contents'}}>
                                            <NodeContainer>{node}</NodeContainer>
                                            {nodeIndex < 2 && <ArrowNode direction="down" length={18}/>}
                                        </Box>
                                    ))
                                    : branch}
                            </Box>
                        </Box>
                    ))}
                </Box>
            </Box>
        );
    }

    return (
        <Box sx={graphRowStyles}>
            {/* Access is both the summary and the entry point for concrete
                authorized resources, so authorization has one work surface. */}
            <NodeContainer>
                {accessNode}
            </NodeContainer>

            <ArrowNode direction="forward" />

            <NodeContainer>
                {botNode}
            </NodeContainer>

            <ArrowNode direction="forward" />

            {/* Fork: @tb and @cc branches */}
            <Box
                sx={{
                    display: 'flex',
                    flexDirection: 'column',
                    gap: 2,
                    borderLeft: '2px solid',
                    borderColor: 'divider',
                    pl: 2,
                }}
            >
                {/* @tb: SmartGuide agent → model */}
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>{smartGuideBranch}</Box>

                {/* @cc: Claude Code agent → profile (default or a claude_code profile) */}
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>{claudeCodeBranch}</Box>
            </Box>
        </Box>
    );
};

export default RemoteControlGraph;
