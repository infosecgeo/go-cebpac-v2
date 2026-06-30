export const numberFormatter = new Intl.NumberFormat('en-US');

export function formatNumber(value) {
    return numberFormatter.format(Number.isFinite(Number(value)) ? Number(value) : 0);
}

export function formatPercentage(value, total = 100) {
    if (!total) {
        return '0%';
    }
    return `${Math.round((Number(value) / Number(total)) * 100)}%`;
}

export function formatDuration(milliseconds) {
    if (!Number.isFinite(milliseconds) || milliseconds <= 0) {
        return '—';
    }
    if (milliseconds < 1000) {
        return `${Math.round(milliseconds)} ms`;
    }
    const totalSeconds = Math.round(milliseconds / 1000);
    const minutes = Math.floor(totalSeconds / 60);
    const seconds = totalSeconds % 60;
    if (!minutes) {
        return `${seconds}s`;
    }
    return `${minutes}m ${seconds}s`;
}

export function formatTimestamp(value) {
    if (!value) {
        return '—';
    }
    const date = typeof value === 'string' || typeof value === 'number' ? new Date(value) : value;
    return Number.isNaN(date.getTime()) ? '—' : date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

export function formatLatency(value) {
    return Number.isFinite(value) ? `${Math.round(value)} ms` : '—';
}

export function formatSystemValue(value, suffix = '') {
    return Number.isFinite(value) ? `${Number(value).toFixed(1)}${suffix}` : '—';
}

export function maskCard(cardLine = '') {
    const cardNumber = cardLine.split('|')[0] || '';
    if (cardNumber.length < 8) {
        return cardLine;
    }
    return `${cardNumber.slice(0, 6)}••••••${cardNumber.slice(-4)}`;
}
