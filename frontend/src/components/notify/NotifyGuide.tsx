import CodeBlock from '@/components/CodeBlock';
import {getDisplayOrigin} from '@/utils/protocol';
import {Box, Stack, Typography} from '@mui/material';
import {useTranslation} from 'react-i18next';

// NotifyGuide is the body of the IM Notify usage guide — it teaches an operator
// how to drive a bot's chat via the authenticated bot-interaction API, embedded
// in the product (ux-principles #8). It is organized around the three questions
// an integrator actually asks (ux-principles #1):
//
//   1. Who can call this, and how do I authenticate?
//   2. What exactly do I send?  (concrete curl + JSON — principle #5)
//   3. Where do I get the stable target UUID the body requires?
//
// Auth reuses the operator's existing user token — no new credential — so the
// guide says so plainly rather than inventing a "notify token" story (see
// .design/bot-interaction-api.md §3.1). The base URL is the operator's own
// origin so the curl is copy-pasteable as-is (principle #11 — hand over the
// artifact for the next action).
const NotifyGuide: React.FC = () => {
    const {t} = useTranslation();
    const origin = getDisplayOrigin();

    // Concrete, copy-pasteable curl. <BOT_UUID> and <TARGET_UUID> are left as
    // placeholders the operator fills from Delivery targets + the per-bot chats
    // list — the guide points there explicitly in section 3.
    const curl = `curl -X POST ${origin}/api/v1/bots/<BOT_UUID>/notify \\
  -H "Authorization: Bearer <USER_TOKEN>" \\
  -H "Content-Type: application/json" \\
  -d '{
    "target": {"kind": "direct_chat", "id": "<TARGET_UUID>"},
    "title": "Build #412 failed",
    "body": "main branch is red",
    "level": "warn"
  }'`;

    const jsonBody = `{
  "target": {"kind": "direct_chat", "id": "<TARGET_UUID>"}, // required
  "title": "Build #412 failed",  // optional
  "body": "main branch is red",  // required
  "level": "info"                // optional: info | warn | error
}`;

    return (
        <Stack spacing={2.5}>
            {/* 1. Who can call this / auth */}
            <Box>
                <Typography variant="subtitle2" sx={{fontWeight: 600, mb: 0.5}}>
                    {t('notify.guide.auth.title', {defaultValue: '1. Authenticate with your user token'})}
                </Typography>
                <Typography variant="body2" sx={{color: 'text.secondary'}}>
                    {t('notify.guide.auth.body', {
                        defaultValue: 'Any integration can drive a bot’s chat. Auth reuses your existing operator user token (the same one this web UI uses) as a Bearer header — no new credential to mint. Interactive prompts (/interact) and one-way notifications (/notify) are separate URLs, so the request shape is the mode.',
                    })}
                </Typography>
            </Box>

            {/* 2. What to send */}
            <Box>
                <Typography variant="subtitle2" sx={{fontWeight: 600, mb: 0.5}}>
                    {t('notify.guide.send.title', {defaultValue: '2. Send a one-way notification'})}
                </Typography>
                <Typography variant="body2" sx={{color: 'text.secondary', mb: 1}}>
                    {t('notify.guide.send.body', {
                        defaultValue: 'POST to /api/v1/bots/{bot}/notify with the bot UUID in the path. A 200 means delivered.',
                    })}
                </Typography>
                <CodeBlock code={curl} language="bash"/>
                <Typography variant="caption" sx={{color: 'text.secondary', mt: 1, display: 'block'}}>
                    {t('notify.guide.send.json', {defaultValue: 'Request body:'})}
                </Typography>
                <CodeBlock code={jsonBody} language="json"/>
            </Box>

            {/* 3. Where to get the internal target UUID */}
            <Box>
                <Typography variant="subtitle2" sx={{fontWeight: 600, mb: 0.5}}>
                    {t('notify.guide.chatid.title', {defaultValue: '3. Copy the target UUID from Delivery targets'})}
                </Typography>
                <Typography variant="body2" sx={{color: 'text.secondary'}}>
                    {t('notify.guide.chatid.body', {
                        defaultValue: 'Each Chat node on this page shows the real platform Chat ID for recognition; its tooltip also shows the stable internal Target UUID required by the API. Use the copy action beside the node, or send a test directly from the probe bench.',
                    })}
                </Typography>
            </Box>
        </Stack>
    );
};

export default NotifyGuide;
