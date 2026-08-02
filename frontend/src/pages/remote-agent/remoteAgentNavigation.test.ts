import { describe, expect, it } from 'vitest';
import {
    getRemoteAgentLandingPlatform,
    isRemoteAgentPlatform,
} from './remoteAgentNavigation';

describe('remote agent platform navigation', () => {
    it('returns the remembered platform when it is supported', () => {
        expect(getRemoteAgentLandingPlatform('lark')).toBe('lark');
    });

    it('falls back to the first supported platform without a memory', () => {
        expect(getRemoteAgentLandingPlatform(null)).toBe('telegram');
    });

    it('rejects stale or unknown remembered platforms', () => {
        expect(isRemoteAgentPlatform('discord')).toBe(false);
        expect(getRemoteAgentLandingPlatform('unknown')).toBe('telegram');
    });
});
