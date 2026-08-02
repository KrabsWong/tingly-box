import { BOT_PLATFORM_IDS } from '@/constants/platformGuides';

export const REMOTE_AGENT_LAST_PLATFORM_KEY = 'tingly.remoteControl.lastPlatform';

export const isRemoteAgentPlatform = (
    platform: string | null | undefined,
): platform is typeof BOT_PLATFORM_IDS[number] =>
    Boolean(platform) && BOT_PLATFORM_IDS.includes(platform as typeof BOT_PLATFORM_IDS[number]);

export const getRemoteAgentLandingPlatform = (rememberedPlatform: string | null) =>
    isRemoteAgentPlatform(rememberedPlatform)
        ? rememberedPlatform
        : BOT_PLATFORM_IDS[0];
