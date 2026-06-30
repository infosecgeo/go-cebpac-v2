import api from '../modules/api.js';
import state from '../modules/state.js';

class AdminService {
    async loadDashboardData() {
        const [statsResult, sessionsResult] = await Promise.allSettled([
            api.get('/admin/stats', { auth: false, retryOnAuth: false }),
            api.get('/admin/sessions', { auth: false, retryOnAuth: false }),
        ]);

        const nextState = {};
        if (statsResult.status === 'fulfilled') {
            nextState.stats = {
                memoryUsageMb: Number(statsResult.value.memoryUsageMb ?? statsResult.value.memory_usage_mb ?? state.getState().stats.memoryUsageMb),
                cpuPercent: Number(statsResult.value.cpuPercent ?? statsResult.value.cpu_percent ?? state.getState().stats.cpuPercent),
                latencyMs: Number(statsResult.value.networkLatencyMs ?? statsResult.value.network_latency_ms ?? state.getState().stats.latencyMs),
            };
        }
        if (sessionsResult.status === 'fulfilled') {
            nextState.user = {
                activeUsers: Number(sessionsResult.value.activeUsers ?? sessionsResult.value.active_users ?? state.getState().user.activeUsers),
                activeSessions: Number(sessionsResult.value.activeSessions ?? sessionsResult.value.active_sessions ?? state.getState().user.activeSessions),
            };
        }
        if (Object.keys(nextState).length) {
            state.setState(nextState, 'admin:dashboard');
        }
        return nextState;
    }
}

export const adminService = new AdminService();
export default adminService;
