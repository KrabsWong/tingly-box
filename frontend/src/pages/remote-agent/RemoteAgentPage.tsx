import { Navigate, useNavigate, useParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useEffect, useMemo, useState } from 'react';
import { PlatformPicker } from '@/components/bot';
import { BOT_PLATFORM_IDS, PLATFORM_BRAND_ICONS, platformDisplayName, usePlatformGuide } from '@/constants/platformGuides';
import { api, enrichBotsWithCapabilities } from '@/services/api';
import { capabilityEnabled, countBotsByPlatform } from '@/types/bot';
import PlatformRemoteAgentPage from './PlatformRemoteAgentPage';
import {
    getRemoteAgentLandingPlatform,
    isRemoteAgentPlatform,
    REMOTE_AGENT_LAST_PLATFORM_KEY,
} from './remoteAgentNavigation';

const readRememberedPlatform = () => {
    try {
        return window.localStorage.getItem(REMOTE_AGENT_LAST_PLATFORM_KEY);
    } catch {
        return null;
    }
};

const rememberPlatform = (platform: string) => {
    try {
        window.localStorage.setItem(REMOTE_AGENT_LAST_PLATFORM_KEY, platform);
    } catch {
        // Storage can be unavailable in privacy-restricted webviews. The
        // deterministic first-platform fallback still keeps navigation usable.
    }
};

export const RemoteAgentEntryRedirect = () => (
    <Navigate
        to={`/remote-agent/${getRemoteAgentLandingPlatform(readRememberedPlatform())}`}
        replace
    />
);

// RemoteAgentPage is the nav-facing entry for the Remote Control purpose:
// ONE sidebar row (under the "Remote" rail icon, alongside Bots and IM
// Notify), with platform selection moved in-page — a grid of picker tiles
// above the page content, instead of nine separate sidebar rows. The routes
// it switches between (/remote-agent/:platform) are unchanged — deep links
// and the bot table purpose chip still work. PlatformRemoteAgentPage itself is
// untouched: same guide, add, and pairing behavior it already had.
const RemoteAgentPage = () => {
    const { platform: routePlatform } = useParams<{ platform: string }>();
    const navigate = useNavigate();
    const { t } = useTranslation();
    const platform = isRemoteAgentPlatform(routePlatform)
        ? routePlatform
        : getRemoteAgentLandingPlatform(readRememberedPlatform());
    const platformName = usePlatformGuide(platform)?.name || platform;

    // An explicit, valid deep link wins and becomes the next sidebar landing
    // platform. Storage is only a re-entry convenience, never route state.
    useEffect(() => {
        if (!isRemoteAgentPlatform(routePlatform)) return;
        rememberPlatform(routePlatform);
    }, [routePlatform]);

    // Active/total per platform, for the tab subtitles — mirrors what the
    // old per-platform sidebar rows showed.
    const [counts, setCounts] = useState<Record<string, { active: number; total: number }>>({});
    useEffect(() => {
        let cancelled = false;
        api.getImBotSettingsList().then(async (data) => {
            if (cancelled || !data?.success || !Array.isArray(data.settings)) return;
            const bots = await enrichBotsWithCapabilities(data.settings);
            if (cancelled) return;
            setCounts(countBotsByPlatform(bots, (bot) => Boolean(bot.enabled) && capabilityEnabled(bot, 'remote_control')));
        }).catch(() => {});
        return () => { cancelled = true; };
    }, [platform]);

    const pickerItems = useMemo(() => BOT_PLATFORM_IDS.map((id) => {
        const BrandIcon = PLATFORM_BRAND_ICONS[id];
        const c = counts[id];
        return {
            id,
            label: platformDisplayName(id, t),
            icon: <BrandIcon size={20} grayscale/>,
            activeIcon: <BrandIcon size={20} grayscale={false}/>,
            subtitle: c && c.total > 0 ? t('bots.activeCount', { defaultValue: 'active {{active}} / {{total}}', active: c.active, total: c.total }) : undefined,
        };
    }), [t, counts]);

    const platformPicker = (
        <PlatformPicker
                items={pickerItems}
                value={platform}
                onChange={(next) => {
                    rememberPlatform(next);
                    navigate(`/remote-agent/${next}`);
                }}
        />
    );

    if (!isRemoteAgentPlatform(routePlatform)) {
        return <Navigate to={`/remote-agent/${platform}`} replace />;
    }

    return (
        <PlatformRemoteAgentPage
            platformId={platform}
            platformName={platformName}
            platformPicker={platformPicker}
        />
    );
};

export default RemoteAgentPage;
