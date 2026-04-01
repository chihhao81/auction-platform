import { describe, it, expect } from 'vitest';
import { ensureUTC, sortAuctions } from '../utils/auctionUtils';

describe('auctionUtils', () => {
    describe('ensureUTC', () => {
        it('should return null if dateStr is null', () => {
            expect(ensureUTC(null)).toBeNull();
        });

        it('should return the same string if it already contains Z', () => {
            const dateStr = '2024-03-24T00:00:00Z';
            expect(ensureUTC(dateStr)).toBe(dateStr);
        });

        it('should append Z and replace space with T if numeric space format', () => {
            const dateStr = '2024-03-24 00:00:00';
            expect(ensureUTC(dateStr)).toBe('2024-03-24T00:00:00Z');
        });
    });

    describe('sortAuctions', () => {
        it('should sort auctions by status (active < upcoming < ended)', () => {
            const auctions = [
                { id: 1, status: 'ended', endTime: '2024-03-24T12:00:00Z', createdAt: '2024-03-24T10:00:00Z' },
                { id: 2, status: 'active', endTime: '2024-03-24T12:00:00Z', createdAt: '2024-03-24T10:00:00Z' },
                { id: 3, status: 'upcoming', endTime: '2024-03-24T12:00:00Z', createdAt: '2024-03-24T10:00:00Z' }
            ];
            const sorted = sortAuctions(auctions);
            expect(sorted[0].status).toBe('active');
            expect(sorted[1].status).toBe('upcoming');
            expect(sorted[2].status).toBe('ended');
        });

        it('should sort by createdAt if status is the same', () => {
            const auctions = [
                { id: 1, status: 'active', endTime: '2024-03-24T12:00:00Z', createdAt: '2024-03-24T11:00:00Z' },
                { id: 2, status: 'active', endTime: '2024-03-24T12:00:00Z', createdAt: '2024-03-24T10:00:00Z' }
            ];
            const sorted = sortAuctions(auctions);
            expect(sorted[0].id).toBe(2);
            expect(sorted[1].id).toBe(1);
        });
    });
});
