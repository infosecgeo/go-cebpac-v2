import { clone } from '../utils/helpers.js';

const initialState = {
    ui: {
        theme: 'dark',
        loadingCount: 0,
    },
    auth: {
        accessToken: null,
        refreshToken: null,
        expiresAt: null,
        authenticated: false,
        refreshing: false,
    },
    user: {
        profile: null,
        activeUsers: 0,
        activeSessions: 0,
        preferences: {},
    },
    socket: {
        status: 'disconnected',
        connected: false,
        reconnectAttempts: 0,
        lastConnectedAt: null,
        latencyMs: null,
    },
    processing: {
        mode: 'manual',
        running: false,
        total: 0,
        currentIndex: 0,
        progress: 0,
        queueLength: 0,
        successCount: 0,
        failedCount: 0,
        retryCount: 0,
        currentTask: 'Idle',
        currentProxy: 'Waiting for backend telemetry',
        currentCard: null,
        estimatedMs: 0,
        averageDurationMs: 0,
        startedAt: null,
        completedAt: null,
    },
    stats: {
        latencyMs: null,
        memoryUsageMb: null,
        cpuPercent: null,
    },
    results: [],
};

function isPlainObject(value) {
    return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function mergeState(base, patch) {
    if (Array.isArray(patch)) {
        return patch.map((item) => (isPlainObject(item) || Array.isArray(item) ? clone(item) : item));
    }
    if (!isPlainObject(patch)) {
        return patch;
    }
    const next = { ...(base || {}) };
    Object.entries(patch).forEach(([key, value]) => {
        const current = base?.[key];
        next[key] = isPlainObject(value) && isPlainObject(current) ? mergeState(current, value) : (isPlainObject(value) || Array.isArray(value) ? clone(value) : value);
    });
    return next;
}

function shallowEqual(left, right) {
    if (Object.is(left, right)) {
        return true;
    }
    if (!left || !right || typeof left !== 'object' || typeof right !== 'object') {
        return false;
    }
    const leftKeys = Object.keys(left);
    const rightKeys = Object.keys(right);
    if (leftKeys.length !== rightKeys.length) {
        return false;
    }
    return leftKeys.every((key) => Object.is(left[key], right[key]));
}

class StateStore extends EventTarget {
    constructor(state) {
        super();
        this.state = clone(state);
    }

    getState() {
        return this.state;
    }

    setState(updater, action = 'state:update') {
        const patch = typeof updater === 'function' ? updater(clone(this.state)) : updater;
        if (!patch) {
            return this.state;
        }
        const previousState = this.state;
        this.state = mergeState(this.state, patch);
        this.dispatchEvent(new CustomEvent('change', {
            detail: {
                action,
                previousState,
                state: this.state,
            },
        }));
        return this.state;
    }

    subscribe(listener, selector = (state) => state) {
        let selected = selector(this.state);
        const handler = (event) => {
            const nextSelected = selector(event.detail.state);
            const changed = isPlainObject(nextSelected) && isPlainObject(selected)
                ? !shallowEqual(selected, nextSelected)
                : !Object.is(selected, nextSelected);

            if (changed) {
                const previous = selected;
                selected = nextSelected;
                listener(nextSelected, previous, event.detail);
            }
        };
        this.addEventListener('change', handler);
        return () => this.removeEventListener('change', handler);
    }

    updateLoading(delta) {
        const current = this.state.ui.loadingCount;
        const nextCount = Math.max(current + delta, 0);
        this.setState({ ui: { loadingCount: nextCount } }, 'ui:loading');
        if (typeof document !== 'undefined') {
            document.body.dataset.loading = nextCount > 0 ? 'true' : 'false';
        }
    }

    resetProcessing({ mode = 'manual', total = 0 }) {
        this.setState({
            processing: {
                mode,
                running: true,
                total,
                currentIndex: 0,
                progress: 0,
                queueLength: total,
                successCount: 0,
                failedCount: 0,
                retryCount: 0,
                currentTask: total ? 'Preparing payment run' : 'Idle',
                currentCard: null,
                estimatedMs: 0,
                averageDurationMs: 0,
                startedAt: new Date().toISOString(),
                completedAt: null,
            },
        }, 'processing:reset');
    }

    updateProcessing(partial, action = 'processing:update') {
        this.setState({ processing: partial }, action);
    }

    incrementRetry() {
        this.setState((current) => ({
            processing: {
                retryCount: current.processing.retryCount + 1,
            },
        }), 'processing:retry');
    }

    addResult(result) {
        this.setState((current) => {
            const results = [...current.results, result];
            const currentIndex = Math.min(current.processing.currentIndex + 1, current.processing.total || 1);
            const averageDurationMs = current.processing.currentIndex === 0
                ? result.durationMs || 0
                : ((current.processing.averageDurationMs * current.processing.currentIndex) + (result.durationMs || 0)) / currentIndex;
            const remaining = Math.max((current.processing.total || currentIndex) - currentIndex, 0);
            return {
                results,
                processing: {
                    currentIndex,
                    queueLength: remaining,
                    progress: current.processing.total ? Math.round((currentIndex / current.processing.total) * 100) : 100,
                    successCount: current.processing.successCount + (result.status === 'live' ? 1 : 0),
                    failedCount: current.processing.failedCount + (result.status === 'dead' ? 1 : 0),
                    averageDurationMs,
                    estimatedMs: averageDurationMs * remaining,
                },
            };
        }, 'results:add');
    }

    clearResults() {
        this.setState({ results: [] }, 'results:clear');
    }

    completeProcessing() {
        this.setState({
            processing: {
                running: false,
                currentTask: 'Idle',
                completedAt: new Date().toISOString(),
                currentCard: null,
            },
        }, 'processing:complete');
    }
}

export const state = new StateStore(initialState);
export default state;
