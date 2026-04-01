export const ensureUTC = (dateStr) => {
    if (!dateStr) return null;
    if (dateStr.includes('Z')) return dateStr;
    return dateStr.replace(' ', 'T') + 'Z';
};

export const sortAuctions = (auctionsList) => {
    return [...auctionsList].sort((a, b) => {
        const statusOrder = { 'active': 1, 'upcoming': 2, 'ended': 3 };
        if (statusOrder[a.status] !== statusOrder[b.status]) {
            return statusOrder[a.status] - statusOrder[b.status];
        }
        return new Date(a.createdAt) - new Date(b.createdAt);
    });
};
