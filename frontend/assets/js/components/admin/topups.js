import api from '../../modules/api.js';

export class AdminTopups {
    constructor({ toast, modal }) {
        this.toast = toast;
        this.modal = modal;
        this.container = null;
        this.topups = [];
        this.statusFilter = 'all';
    }

    async mount(container) {
        this.container = container;
        await this.loadTopups();
        this.render();
        this.attachEventListeners();
    }

    async loadTopups() {
        try {
            let url = '/api/v1/admin/topups';
            if (this.statusFilter !== 'all') {
                url += `?status=${this.statusFilter}`;
            }
            const response = await api.get(url);
            this.topups = response.topup_requests || [];
        } catch (error) {
            console.error('Failed to load topups:', error);
            this.topups = [];
        }
    }

    render() {
        if (!this.container) return;

        this.container.innerHTML = `
            <h2 class="section-title">Top-up Requests</h2>
            
            <div class="admin-table-container">
                <div class="table-header">
                    <h3 class="table-title">All Top-up Requests</h3>
                    <div class="table-actions">
                        <select id="status-filter" class="search-input" style="min-width: 150px;">
                            <option value="all" ${this.statusFilter === 'all' ? 'selected' : ''}>All Status</option>
                            <option value="pending" ${this.statusFilter === 'pending' ? 'selected' : ''}>Pending</option>
                            <option value="approved" ${this.statusFilter === 'approved' ? 'selected' : ''}>Approved</option>
                            <option value="denied" ${this.statusFilter === 'denied' ? 'selected' : ''}>Denied</option>
                        </select>
                    </div>
                </div>
                
                ${this.topups.length > 0 ? `
                    <table class="data-table">
                        <thead>
                            <tr>
                                <th>User</th>
                                <th>Amount</th>
                                <th>Status</th>
                                <th>Payment Receipt</th>
                                <th>Requested</th>
                                <th>Actions</th>
                            </tr>
                        </thead>
                        <tbody>
                            ${this.topups.map(topup => `
                                <tr>
                                    <td>${topup.user_email || topup.user_id}</td>
                                    <td>${topup.amount} credits</td>
                                    <td>
                                        <span class="status-badge status-${topup.status}">
                                            ${topup.status}
                                        </span>
                                    </td>
                                    <td>
                                        ${topup.payment_receipt_url ? 
                                            `<a href="${topup.payment_receipt_url}" target="_blank" class="link">View</a>` : 
                                            'N/A'}
                                    </td>
                                    <td>${topup.created_at ? new Date(topup.created_at).toLocaleString() : 'N/A'}</td>
                                    <td>
                                        ${topup.status === 'pending' ? `
                                            <div class="action-buttons">
                                                <button 
                                                    class="btn btn-sm btn-success approve-topup-btn" 
                                                    data-topup-id="${topup.id}"
                                                >
                                                    Approve
                                                </button>
                                                <button 
                                                    class="btn btn-sm btn-danger deny-topup-btn" 
                                                    data-topup-id="${topup.id}"
                                                >
                                                    Deny
                                                </button>
                                            </div>
                                        ` : '-'}
                                    </td>
                                </tr>
                            `).join('')}
                        </tbody>
                    </table>
                ` : `
                    <div class="empty-state">
                        <div class="empty-state-icon">💰</div>
                        <div class="empty-state-title">No top-up requests</div>
                        <p>Top-up requests will appear here when users submit them via Telegram.</p>
                    </div>
                `}
            </div>
        `;
    }

    attachEventListeners() {
        const statusFilter = this.container.querySelector('#status-filter');
        if (statusFilter) {
            statusFilter.addEventListener('change', async (e) => {
                this.statusFilter = e.target.value;
                await this.loadTopups();
                this.render();
                this.attachEventListeners();
            });
        }

        const approveButtons = this.container.querySelectorAll('.approve-topup-btn');
        approveButtons.forEach(btn => {
            btn.addEventListener('click', async () => {
                const topupId = btn.dataset.topupId;
                btn.disabled = true;
                btn.textContent = 'Approving...';
                // Note: Actual approval should be done via Telegram bot callback
                // This is just a UI placeholder
                this.toast.show({
                    title: 'Info',
                    message: 'Top-up approvals should be done via Telegram bot',
                    type: 'info',
                });
                btn.disabled = false;
                btn.textContent = 'Approve';
            });
        });

        const denyButtons = this.container.querySelectorAll('.deny-topup-btn');
        denyButtons.forEach(btn => {
            btn.addEventListener('click', async () => {
                const topupId = btn.dataset.topupId;
                btn.disabled = true;
                btn.textContent = 'Denying...';
                // Note: Actual denial should be done via Telegram bot callback
                this.toast.show({
                    title: 'Info',
                    message: 'Top-up denials should be done via Telegram bot',
                    type: 'info',
                });
                btn.disabled = false;
                btn.textContent = 'Deny';
            });
        });
    }
}
