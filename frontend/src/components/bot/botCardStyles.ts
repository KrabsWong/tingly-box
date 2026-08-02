// Shared shell + status-chip styling for BotCard and RemoteAgentBotCard —
// the two "twin" per-bot cards (see RemoteAgentBotCard's own comment). The
// established resource-row look is a plain bordered container plus an
// explicit On/Off chip (ApiKeyTable, RulesPage, ToolCard) — no dashed
// border, no grayscale filter, no monospace names, which is what this
// replaced. What it keeps from the original is the "disabled" affordance:
// when a bot is off (or a purpose is unmounted), a faint diagonal hatch is
// laid over the card so an inactive row is unmistakable at a glance. The
// overlay is pointer-transparent, so the card stays fully interactive (you
// can still toggle it back on / edit through the hatch).
import { inactiveHatchSx } from '../nodes/styles';

export const botCardSx = (active: boolean) => ({
    position: 'relative' as const,
    bgcolor: 'background.paper',
    border: '1px solid',
    borderColor: 'divider',
    borderRadius: 2,
    boxShadow: 'none',
    ...(active ? {} : inactiveHatchSx),
});

export const statusChipSx = { height: 22, minWidth: 40 } as const;
