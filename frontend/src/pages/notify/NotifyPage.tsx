import EmptyState from '@/components/EmptyState';
import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import CollapsibleGuide from '@/components/remote-control/CollapsibleGuide';
import NotifyGuide from '@/components/notify/NotifyGuide';
import BotNotifyGroup from '@/components/notify/BotNotifyGroup';
import { api } from '@/services/api';
import type { BotSettings } from '@/types/bot';
import { Stack } from '@mui/material';
import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

// NotifyPage opens up the authenticated bot-interaction API: it teaches how to
// call it (top guide) and, per bot, shows the chats that bot can reach as an
// always-expanded table — each chat row carries the concrete chat_id that
// POST /api/v1/bots/:bot/notify needs in its body, with copy + send-test inline
// (no extra click to reach the work surface). The bot's enabled switch is the
// on/off for whether it can be driven.
//
// This page is no longer about "which scenario routes point at this bot" (that
// was the old read-only framing, surfaced as a misleading "No routes" chip) —
// it answers the operator's actual question: "what can I send to, right now?"
// See .design/bot-interaction-api.md and ux-principles #1/#5/#11.
const NotifyPage = () => {
    const { t } = useTranslation();
    const [bots, setBots] = useState<BotSettings[]>([]);
    const [loading, setLoading] = useState(true);
	const [toggling,setToggling]=useState<string|null>(null);

    const loadBots = useCallback(async () => {
        try {
            setLoading(true);
            const data = await api.getImBotSettingsList();
            if (data?.success && Array.isArray(data.settings)) {
				const enriched=await Promise.all(data.settings.map(async(bot:BotSettings)=>{try{const result=await api.listBotCapabilities(bot.uuid!);return {...bot,capabilities:result.capabilities||[]}}catch{return {...bot,capabilities:[]}}}));
				setBots(enriched);
            }
        } catch (err) {
            console.error('Failed to load bot settings:', err);
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        loadBots();
    }, [loadBots]);

    // Toggle is the shared control-plane action (POST /imbot-settings/:uuid/
    // toggle): it starts/stops the bot's channel, which governs whether notify
    // can reach it. On success we patch only the toggled bot in state rather
    // than re-fetching the whole list, so sibling panels aren't churned. The
    // hook owns the toast + in-flight UUID and is shared with the Bots pages.
	const handleToggle=useCallback(async(uuid:string,enabled:boolean)=>{setToggling(uuid);try{await api.setBotCapability(uuid,'notify',enabled);await loadBots()}finally{setToggling(null)}},[loadBots]);

    return (
        <PageLayout loading={loading}>
            {/* Usage guide — embedded education (ux-principles #8). Collapsed by
                default so the bots + chats (the work surface) are the visual
                anchor; the guide is one click away whenever needed. */}
            <CollapsibleGuide
                platformName={t('notify.title', { defaultValue: 'IM Notify' })}
                platformGuide={<NotifyGuide />}
            />
            <UnifiedCard
                title={t('notify.title', { defaultValue: 'IM Notify' })}
                subtitle={t('notify.subtitle', {
                    defaultValue: 'Send one-way notifications to any chat your bots can reach.',
                })}
                size="full"
                sx={{ mb: 2 }}
                titleHeadingLevel={1}
            >
                {bots.length === 0 ? (
                    <EmptyState
                        title={t('notify.emptyTitle', { defaultValue: 'No bots connected yet' })}
                        description={t('notify.emptyDescription', { defaultValue: 'Connect a bot on the Bots page first, then come back here to send it notifications.' })}
                    />
                ) : (
                    <Stack spacing={1.5}>
                        {bots.map((bot) => (
                            <BotNotifyGroup
                                key={bot.uuid}
                                bot={bot}
								onToggle={(uuid,enabled) => handleToggle(uuid,enabled)}
								isToggling={toggling===bot.uuid}
                            />
                        ))}
                    </Stack>
                )}
            </UnifiedCard>
        </PageLayout>
    );
};

export default NotifyPage;
