import { render, screen } from '@testing-library/react';
import { vi, describe, it, expect } from 'vitest';
import React from 'react';
import Home from '../pages/Home';
import { useAuction } from '../context/AuctionContext';
import { useAuth } from '../context/AuthContext';

// Mock the contexts
vi.mock('../context/AuctionContext', () => ({
    useAuction: vi.fn()
}));

vi.mock('../context/AuthContext', () => ({
    useAuth: vi.fn()
}));

describe('Home Component', () => {
    it('renders loading state when auctionLoading is true', () => {
        useAuction.mockReturnValue({
            auctions: [],
            loading: true,
            fetchAuctions: vi.fn(),
            placeBid: vi.fn(),
        });
        useAuth.mockReturnValue({ user: null });

        render(<Home />);
        expect(screen.getByText(/讀取中/i)).toBeInTheDocument();
    });

    it('renders no products message when auctions list is empty', () => {
        useAuction.mockReturnValue({
            auctions: [],
            loading: false,
            fetchAuctions: vi.fn(),
            placeBid: vi.fn(),
        });
        useAuth.mockReturnValue({ user: { id: 'test-user' } });

        render(<Home />);
        expect(screen.getByText(/目前沒有競標中的商品/i)).toBeInTheDocument();
    });

    it('renders auction cards when auctions are available', () => {
        const mockAuctions = [
            {
                id: 1,
                name: '測試商品',
                description: '測試描述',
                image: 'test.jpg',
                startPrice: 100,
                minIncrement: 10,
                startTime: new Date().toISOString(),
                endTime: new Date(Date.now() + 3600000).toISOString(),
                status: 'active',
                bids: []
            }
        ];
        useAuction.mockReturnValue({
            auctions: mockAuctions,
            loading: false,
            fetchAuctions: vi.fn(),
            placeBid: vi.fn(),
        });
        useAuth.mockReturnValue({ user: { id: 'test-user' } });

        render(<Home />);
        expect(screen.getByText('測試商品')).toBeInTheDocument();
        expect(screen.getByText('$100')).toBeInTheDocument();
    });
});
