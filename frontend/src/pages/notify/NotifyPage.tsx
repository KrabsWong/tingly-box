import EmptyState from '@/components/EmptyState';
import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import GuideDrawer from '@/components/remote-control/GuideDrawer';
import NotifyGuide from '@/components/notify/NotifyGuide';
import BotNotifyGroup from '@/components/notify/BotNotifyGroup';
import { PlatformPicker } from '@/components/bot';
import { HelpOutline, ListAlt } from '@/components/icons';
import { BOT_PLATFORM_IDS, PLATFORM_BRAND_ICONS, platformDisplayName } from '@/constants/platformGuides';
import { api } from '@/services/api';
import type { BotSettings } from '@/types/bot';
import { capabilityEnabled, countBotsByPlatform } from '@/types/bot';
import { notify } from '@/utils/notify';
import { Button, Stack } from '@mui/material';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';

// NotifyPage opens up the authenticated bot-interaction API: it teaches how to
// call it (top guide) and, per bot, shows the chats that bot can reach as an
// always-expanded graph — each Chat node pairs the concrete platform id with
// the stable internal target UUID required by /notify, with test actions inline
// (no extra click to reach the work surface). The bot's enabled switch is the
// on/off for whether it can be driven.
//
// This page is no longer about "which scenario routes point at this bot" (that
// was the old read-only framing, surfaced as a misleading "No routes" chip) —
// it answers the operator's actual question: "what can I send to, right now?"
// See .design/bot-interaction-api.md and ux-principles #1/#5/#11.
const NotifyPage = () => {
    const { t } = useTranslation();
    const [searchParams, setSearchParams] = useSearchParams();
    const selectedPlatform = searchParams.get('platform') || 'all';
    const [bots, setBots] = useState<BotSettings[]>([]);
    const [loading, setLoading] = useState(true);
    const [toggling, setToggling] = useState<string | null>(null);
    const [guideOpen, setGuideOpen] = useState(false);

    const loadBots = useCallback(async () => {
        try {
            setLoading(true);
            const data = await api.getImBotSettingsList();
            if (data?.success && Array.isArray(data.settings)) {
                const enriched = await Promise.all(data.settings.map(async (bot: BotSettings) => {
                    try {
                        const result = await api.listBotCapabilities(bot.uuid!);
                        return {...bot, capabilities: result.capabilities || []};
                    } catch {
                        return {...bot, capabilities: []};
                    }
                }));
                setBots(enriched);
            }
        } catch (err) {
            console.error('Failed to load bot settings:', err);
            notify.error(err instanceof Error ? err.message : t('notify.loadFailed', {defaultValue: 'Failed to load Notify targets'}));
        } finally {
            setLoading(false);
        }
    }, [t]);

    useEffect(() => {
        loadBots();
    }, [loadBots]);

    // Notify is an explicit capability. Its lifecycle is reconciled by the
    // backend: enabling it starts the Bot, and disabling the last capability
    // stops the Bot.
    const handleToggle = useCallback(async (uuid: string, enabled: boolean) => {
        setToggling(uuid);
        try {
            await api.setBotCapability(uuid, 'notify', enabled);
            await loadBots();
        } catch (toggleError) {
            notify.error(toggleError instanceof Error ? toggleError.message : t('notify.toggleFailed', {defaultValue: 'Failed to update Notify'}));
        } finally {
            setToggling(null);
        }
    }, [loadBots]);

    // Platform picker — the same top-level platform tiles the Bots overview
    // uses (shared PlatformPicker component + ?platform= URL param), so Remote
    // surfaces navigate identically. Only platforms that actually have bots
    // get a tile here: this page drives existing bots, it doesn't create them.
    const isNotifyActive = useCallback((bot: BotSettings) => Boolean(bot.enabled) && capabilityEnabled(bot, 'notify'), []);
    const platformCounts = useMemo(() => countBotsByPlatform(bots, isNotifyActive), [bots, isNotifyActive]);

    const pickerItems = useMemo(() => {
        // Defined inside the memo so its only dep (t) is covered by the array.
        const countLabel = (active: number, total: number): string | undefined =>
            total > 0 ? t('bots.activeCount', { defaultValue: 'active {{active}} / {{total}}', active, total }) : undefined;
        return [
            {
                id: 'all',
                label: t('bots.overview.allPlatforms', { defaultValue: 'All' }),
                icon: <ListAlt sx={{fontSize: 20, color: 'text.disabled'}}/>,
                activeIcon: <ListAlt sx={{fontSize: 20, color: 'primary.main'}}/>,
                subtitle: countLabel(bots.filter(isNotifyActive).length, bots.length),
            },
            ...BOT_PLATFORM_IDS.filter((id) => platformCounts[id]).map((id) => {
                const BrandIcon = PLATFORM_BRAND_ICONS[id];
                const c = platformCounts[id];
                return {
                    id,
                    label: platformDisplayName(id, t),
                    icon: <BrandIcon size={20} grayscale/>,
                    activeIcon: <BrandIcon size={20} grayscale={false}/>,
                    subtitle: c ? countLabel(c.active, c.total) : undefined,
                };
            }),
        ];
    }, [t, bots, platformCounts, isNotifyActive]);

    const selectPlatform = useCallback((id: string) => {
        // Functional update keeps this callback stable across URL changes
        // (no searchParams dep → PlatformPicker isn't re-rendered per nav).
        setSearchParams(prev => {
            const next = new URLSearchParams(prev);
            if (id === 'all') next.delete('platform');
            else next.set('platform', id);
            return next;
        });
    }, [setSearchParams]);

    const filteredBots = useMemo(
        () => selectedPlatform === 'all' ? bots : bots.filter(b => b.platform === selectedPlatform),
        [bots, selectedPlatform]
    );

    return (
        <PageLayout
            loading={loading}
            title={t('notify.title', {defaultValue: 'IM Notify'})}
            subtitle={t('notify.subtitle', {defaultValue: 'Authorize a target, send through the production path, and see whether delivery worked.'})}
        >
            {/* Platform selection changes the work surface. The platform-agnostic
                API guide lives on that surface as a secondary action. */}
            <PlatformPicker items={pickerItems} value={selectedPlatform} onChange={selectPlatform} />
            <UnifiedCard
                title={t('notify.targetsTitle', { defaultValue: 'Delivery targets' })}
                subtitle={t('notify.targetsSubtitle', {defaultValue: 'Direct Chats and Groups observed by your connected bots.'})}
                size="full"
                sx={{ mb: 2 }}
                titleHeadingLevel={2}
                rightAction={(
                    <Button
                        variant="outlined"
                        size="small"
                        startIcon={<HelpOutline />}
                        onClick={() => setGuideOpen(true)}
                        sx={{ whiteSpace: 'nowrap' }}
                    >
                        {t('notify.guide.action', { defaultValue: 'API guide' })}
                    </Button>
                )}
            >
                {filteredBots.length === 0 ? (
                    bots.length === 0 ? (
                        <EmptyState
                            title={t('notify.emptyTitle', { defaultValue: 'No bots connected yet' })}
                            description={t('notify.emptyDescription', { defaultValue: 'Connect a bot on the Bots page first, then come back here to send it notifications.' })}
                        />
                    ) : (
                        // Bots exist, just none on the selected platform (e.g. a
                        // stale ?platform= bookmark) — answer that question, not
                        // "do you have any bots at all?" (ux-principles #1).
                        <EmptyState
                            title={t('notify.emptyPlatformTitle', { defaultValue: 'No {{platform}} bots', platform: platformDisplayName(selectedPlatform, t) })}
                            description={t('notify.emptyPlatformDescription', { defaultValue: 'Pick another platform above, or add one on the Bots page.' })}
                        />
                    )
                ) : (
                    <Stack spacing={1.5}>
                        {filteredBots.map((bot) => (
                            <BotNotifyGroup
                                key={bot.uuid}
                                bot={bot}
                                onToggle={handleToggle}
                                isToggling={toggling === bot.uuid}
                            />
                        ))}
                    </Stack>
                )}
            </UnifiedCard>
            <GuideDrawer
                open={guideOpen}
                title={t('notify.guide.title', { defaultValue: 'IM Notify API Guide' })}
                description={t('notify.guide.description', {
                    defaultValue: 'Authentication, request examples, and target IDs',
                })}
                onClose={() => setGuideOpen(false)}
            >
                <NotifyGuide />
            </GuideDrawer>
        </PageLayout>
    );
};

export default NotifyPage;
