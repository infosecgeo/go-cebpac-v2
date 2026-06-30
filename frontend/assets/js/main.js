import storage from './modules/storage.js';
import state from './modules/state.js';
import { WebSocketClient } from './modules/websocket.js';
import { Dashboard } from './components/dashboard.js';
import { PaymentForm } from './components/paymentForm.js';
import { ProgressTracker } from './components/progressTracker.js';
import { ResultsList } from './components/resultsList.js';
import { ThemeToggle } from './components/themeToggle.js';
import { ToastManager } from './components/toast.js';
import { ModalManager } from './components/modal.js';
import adminService from './services/adminService.js';

function getWebSocketUrl() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    return `${protocol}//${window.location.host}/ws`;
}

function updateConnectionPill(socketState) {
    const pill = document.getElementById('connection-pill');
    const label = document.getElementById('connection-label');
    if (!pill || !label) {
        return;
    }
    pill.classList.remove('status-connected', 'status-disconnected', 'status-error', 'status-reconnecting');
    pill.classList.add(`status-${socketState.status}`);
    const labels = {
        connecting: 'Connecting telemetry',
        connected: 'Telemetry connected',
        disconnected: 'Telemetry offline',
        reconnecting: 'Reconnecting telemetry',
        error: 'Telemetry error',
    };
    label.textContent = labels[socketState.status] || 'Telemetry offline';
}

function bindWebSocketTelemetry(socketClient, toast) {
    socketClient.addEventListener('statuschange', (event) => updateConnectionPill(event.detail));
    socketClient.addEventListener('progress', (event) => {
        const data = event.detail.data || {};
        state.updateProcessing({
            currentIndex: Number(data.current_index ?? state.getState().processing.currentIndex),
            total: Number(data.total ?? state.getState().processing.total),
            progress: Number(data.percentage ?? state.getState().processing.progress),
            successCount: Number(data.success_count ?? state.getState().processing.successCount),
            failedCount: Number(data.failed_count ?? state.getState().processing.failedCount),
            retryCount: Number(data.retry_count ?? state.getState().processing.retryCount),
            queueLength: Number(data.queue_length ?? state.getState().processing.queueLength),
            currentTask: data.current_task ?? state.getState().processing.currentTask,
            currentProxy: data.current_proxy ?? state.getState().processing.currentProxy,
        }, 'socket:progress');
    });
    socketClient.addEventListener('stats_update', (event) => {
        const data = event.detail.data || {};
        state.setState({
            stats: {
                memoryUsageMb: Number(data.memory_usage_mb ?? state.getState().stats.memoryUsageMb),
                cpuPercent: Number(data.cpu_percent ?? state.getState().stats.cpuPercent),
                latencyMs: Number(data.network_latency_ms ?? state.getState().stats.latencyMs),
            },
            user: {
                activeUsers: Number(data.active_users ?? state.getState().user.activeUsers),
                activeSessions: Number(data.active_sessions ?? state.getState().user.activeSessions),
            },
        }, 'socket:stats');
    });
    socketClient.addEventListener('proxy_change', (event) => {
        state.updateProcessing({ currentProxy: event.detail.data?.current_proxy ?? 'Assigned by backend' }, 'socket:proxy');
    });
    socketClient.addEventListener('task_error', (event) => {
        const message = event.detail.data?.message || 'A background task reported an error.';
        toast.show({ title: 'Task error', message, type: 'error' });
    });
}

async function bootstrap() {
    const toast = new ToastManager();
    toast.mount(document.getElementById('toast-root'));

    const modal = new ModalManager();
    modal.mount(document.getElementById('modal-root'));

    const themeToggle = new ThemeToggle();
    themeToggle.mount(document.getElementById('theme-toggle'));

    state.setState({
        auth: {
            ...storage.getAuthTokens(),
            authenticated: Boolean(storage.getAuthTokens().accessToken),
        },
        user: {
            preferences: storage.getUserPreferences(),
        },
        ui: {
            theme: storage.getTheme(),
        },
    }, 'app:hydrate');

    const dashboard = new Dashboard();
    dashboard.mount(document.getElementById('dashboard-mount'));

    const paymentForm = new PaymentForm({ toast, modal });
    paymentForm.mount(document.getElementById('payment-form-mount'));

    const progressTracker = new ProgressTracker();
    progressTracker.mount(document.getElementById('progress-tracker-mount'));

    const resultsList = new ResultsList({ toast, modal });
    resultsList.mount(document.getElementById('results-list-mount'));

    updateConnectionPill(state.getState().socket);
    state.subscribe((socketState) => updateConnectionPill(socketState), (currentState) => currentState.socket);

    const socketClient = new WebSocketClient({
        url: getWebSocketUrl(),
        reconnect: storage.getUserPreferences().reconnectWebSocket !== false,
    });
    bindWebSocketTelemetry(socketClient, toast);
    socketClient.connect();

    try {
        await adminService.loadDashboardData();
    } catch {
        // Dashboard boot should still succeed if admin endpoints are unavailable.
    }
}

if (typeof document !== 'undefined') {
    document.addEventListener('DOMContentLoaded', bootstrap);
}
