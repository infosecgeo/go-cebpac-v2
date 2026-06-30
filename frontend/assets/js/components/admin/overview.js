import api from '../../modules/api.js';

export class AdminOverview {
    constructor({ toast, modal }) {
        this.toast = toast;
        this.modal = modal;
        this.container = null;
        this.stats = null;
    }

    async mount(container) {
        this.container = container;
        await this.loadStats();
        this.render();
    }

    async loadStats() {
        try {
            // TODO: Create a dedicated stats endpoint
            // For now, fetch from individual endpoints
            const [users, licenses, topups] = await Promise.all([
                api.get('/api/v1/admin/users').catch(() => ({ users: [] })),
                api.get('/api/v1/admin/licenses').catch(() => ({ licenses: [] })),
                api.get('/api/v1/admin/topups?status=pending').catch(() => ({ topup_requests: [] })),
            ]);

            this.stats = {
                totalUsers: users.users?.length || 0,
                activeLicenses: licenses.licenses?.filter(l => l.status === 'active').length || 0,
                pendingTopups: topups.topup_requests?.length || 0,
                todayTransactions: 0, // TODO: Get from transactions endpoint
            };
        } catch (error) {
            console.error('Failed to load stats:', error);
            this.stats = {
                totalUsers: 0,
                activeLicenses: 0,
                pendingTopups: 0,
                todayTransactions: 0,
            };
        }
    }

    render() {
        if (!this.container || !this.stats) return;

        this.container.innerHTML = `
            <h2 class="section-title">Dashboard Overview</h2>
            
            <div class="stats-grid">
                <div class="stat-card">
                    <div class="stat-label">Total Users</div>
                    <div class="stat-value">${this.stats.totalUsers}</div>
                </div>
                
                <div class="stat-card">
                    <div class="stat-label">Active Licenses</div>
                    <div class="stat-value">${this.stats.activeLicenses}</div>
                </div>
                
                <div class="stat-card">
                    <div class="stat-label">Pending Top-ups</div>
                    <div class="stat-value">${this.stats.pendingTopups}</div>
                </div>
                
                <div class="stat-card">
                    <div class="stat-label">Today's Transactions</div>
                    <div class="stat-value">${this.stats.todayTransactions}</div>
                </div>
            </div>

            <div class="admin-table-container">
                <div class="table-header">
                    <h3 class="table-title">Recent Activity</h3>
                </div>
                <div class="empty-state">
                    <div class="empty-state-icon">📊</div>
                    <div class="empty-state-title">No recent activity</div>
                    <p>Activity will appear here as users interact with the system.</p>
                </div>
            </div>
        `;
    }
}
