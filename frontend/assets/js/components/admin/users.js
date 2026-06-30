import api from '../../modules/api.js';

export class AdminUsers {
    constructor({ toast, modal }) {
        this.toast = toast;
        this.modal = modal;
        this.container = null;
        this.users = [];
        this.filteredUsers = [];
        this.searchQuery = '';
    }

    async mount(container) {
        this.container = container;
        await this.loadUsers();
        this.render();
        this.attachEventListeners();
    }

    async loadUsers() {
        try {
            const response = await api.get('/api/v1/admin/users');
            this.users = response.users || [];
            this.filteredUsers = [...this.users];
        } catch (error) {
            console.error('Failed to load users:', error);
            this.toast.show({
                title: 'Error',
                message: 'Failed to load users',
                type: 'error',
            });
            this.users = [];
            this.filteredUsers = [];
        }
    }

    render() {
        if (!this.container) return;

        this.container.innerHTML = `
            <h2 class="section-title">User Management</h2>
            
            <div class="admin-table-container">
                <div class="table-header">
                    <h3 class="table-title">All Users</h3>
                    <div class="table-actions">
                        <input 
                            type="text" 
                            id="user-search" 
                            class="search-input" 
                            placeholder="Search by email or license..."
                            value="${this.searchQuery}"
                        />
                    </div>
                </div>
                
                ${this.filteredUsers.length > 0 ? `
                    <table class="data-table">
                        <thead>
                            <tr>
                                <th>Email</th>
                                <th>License</th>
                                <th>Credits</th>
                                <th>Status</th>
                                <th>Telegram</th>
                                <th>Registered</th>
                                <th>Actions</th>
                            </tr>
                        </thead>
                        <tbody>
                            ${this.filteredUsers.map(user => `
                                <tr>
                                    <td>${this.escapeHtml(user.email || 'N/A')}</td>
                                    <td><code>${this.escapeHtml(user.license_key || 'N/A')}</code></td>
                                    <td>${user.credits || 0}</td>
                                    <td>
                                        <span class="status-badge status-${user.registration_status || 'pending'}">
                                            ${user.registration_status || 'pending'}
                                        </span>
                                    </td>
                                    <td>
                                        ${user.telegram_username ? 
                                            `@${this.escapeHtml(user.telegram_username)}` : 
                                            '<span style="opacity: 0.5;">Not linked</span>'}
                                    </td>
                                    <td>${user.created_at ? new Date(user.created_at).toLocaleDateString() : 'N/A'}</td>
                                    <td>
                                        <div class="action-buttons">
                                            <button 
                                                class="btn btn-sm btn-primary edit-user-btn" 
                                                data-user-id="${user.id}"
                                            >
                                                Edit
                                            </button>
                                            <button 
                                                class="btn btn-sm btn-danger delete-user-btn" 
                                                data-user-id="${user.id}"
                                            >
                                                Delete
                                            </button>
                                        </div>
                                    </td>
                                </tr>
                            `).join('')}
                        </tbody>
                    </table>
                ` : `
                    <div class="empty-state">
                        <div class="empty-state-icon">👥</div>
                        <div class="empty-state-title">No users found</div>
                        <p>Users will appear here once they register.</p>
                    </div>
                `}
            </div>
        `;
    }

    attachEventListeners() {
        // Search
        const searchInput = this.container.querySelector('#user-search');
        if (searchInput) {
            searchInput.addEventListener('input', (e) => {
                this.searchQuery = e.target.value;
                this.filterUsers();
            });
        }

        // Edit buttons
        const editButtons = this.container.querySelectorAll('.edit-user-btn');
        editButtons.forEach(btn => {
            btn.addEventListener('click', () => {
                const userId = btn.dataset.userId;
                this.showEditUserModal(userId);
            });
        });

        // Delete buttons
        const deleteButtons = this.container.querySelectorAll('.delete-user-btn');
        deleteButtons.forEach(btn => {
            btn.addEventListener('click', () => {
                const userId = btn.dataset.userId;
                this.confirmDeleteUser(userId);
            });
        });
    }

    filterUsers() {
        const query = this.searchQuery.toLowerCase();
        this.filteredUsers = this.users.filter(user => 
            (user.email && user.email.toLowerCase().includes(query)) ||
            (user.license_key && user.license_key.toLowerCase().includes(query)) ||
            (user.telegram_username && user.telegram_username.toLowerCase().includes(query))
        );
        this.render();
        this.attachEventListeners();
    }

    showEditUserModal(userId) {
        const user = this.users.find(u => u.id === userId);
        if (!user) return;

        this.modal.show({
            title: `Edit User: ${user.email || user.license_key}`,
            content: `
                <form id="edit-user-form" class="form-stack">
                    <div class="form-group">
                        <label for="edit-credits">Credits</label>
                        <input 
                            type="number" 
                            id="edit-credits" 
                            name="credits" 
                            class="form-input"
                            value="${user.credits || 0}"
                            min="0"
                            required
                        />
                    </div>
                    <div class="form-group">
                        <label for="edit-status">Status</label>
                        <select id="edit-status" name="registration_status" class="form-input">
                            <option value="pending" ${user.registration_status === 'pending' ? 'selected' : ''}>Pending</option>
                            <option value="active" ${user.registration_status === 'active' ? 'selected' : ''}>Active</option>
                            <option value="suspended" ${user.registration_status === 'suspended' ? 'selected' : ''}>Suspended</option>
                        </select>
                    </div>
                    <div class="form-actions">
                        <button type="button" class="btn btn-secondary" id="cancel-edit-btn">Cancel</button>
                        <button type="submit" class="btn btn-primary" id="save-user-btn">Save Changes</button>
                    </div>
                </form>
            `,
            onMount: (modalElement) => {
                const form = modalElement.querySelector('#edit-user-form');
                const saveBtn = modalElement.querySelector('#save-user-btn');
                const cancelBtn = modalElement.querySelector('#cancel-edit-btn');

                form.addEventListener('submit', async (e) => {
                    e.preventDefault();
                    await this.saveUser(userId, form, saveBtn);
                });

                cancelBtn.addEventListener('click', () => {
                    this.modal.hide();
                });
            },
        });
    }

    async saveUser(userId, form, saveBtn) {
        const credits = parseInt(form.querySelector('#edit-credits').value);
        const registrationStatus = form.querySelector('#edit-status').value;

        saveBtn.disabled = true;
        saveBtn.textContent = 'Saving...';

        try {
            await api.put(`/api/v1/admin/users/${userId}`, {
                credits,
                registration_status: registrationStatus,
            });

            this.modal.hide();
            this.toast.show({
                title: 'Success',
                message: 'User updated successfully',
                type: 'success',
            });

            await this.loadUsers();
            this.render();
            this.attachEventListeners();
        } catch (error) {
            console.error('Failed to update user:', error);
            this.toast.show({
                title: 'Error',
                message: error.message || 'Failed to update user',
                type: 'error',
            });
        } finally {
            saveBtn.disabled = false;
            saveBtn.textContent = 'Save Changes';
        }
    }

    confirmDeleteUser(userId) {
        const user = this.users.find(u => u.id === userId);
        if (!user) return;

        this.modal.show({
            title: 'Confirm Delete',
            content: `
                <div class="alert alert-warning">
                    <div class="alert-icon">⚠️</div>
                    <div class="alert-content">
                        <p>Are you sure you want to delete user <strong>${this.escapeHtml(user.email || user.license_key)}</strong>?</p>
                        <p>This action cannot be undone.</p>
                    </div>
                </div>
                <div class="form-actions">
                    <button type="button" class="btn btn-secondary" id="cancel-delete-btn">Cancel</button>
                    <button type="button" class="btn btn-danger" id="confirm-delete-btn">Delete User</button>
                </div>
            `,
            onMount: (modalElement) => {
                const confirmBtn = modalElement.querySelector('#confirm-delete-btn');
                const cancelBtn = modalElement.querySelector('#cancel-delete-btn');

                confirmBtn.addEventListener('click', async () => {
                    await this.deleteUser(userId, confirmBtn);
                });

                cancelBtn.addEventListener('click', () => {
                    this.modal.hide();
                });
            },
        });
    }

    async deleteUser(userId, confirmBtn) {
        confirmBtn.disabled = true;
        confirmBtn.textContent = 'Deleting...';

        try {
            await api.delete(`/api/v1/admin/users/${userId}`);

            this.modal.hide();
            this.toast.show({
                title: 'Success',
                message: 'User deleted successfully',
                type: 'success',
            });

            await this.loadUsers();
            this.render();
            this.attachEventListeners();
        } catch (error) {
            console.error('Failed to delete user:', error);
            this.toast.show({
                title: 'Error',
                message: error.message || 'Failed to delete user',
                type: 'error',
            });
            confirmBtn.disabled = false;
            confirmBtn.textContent = 'Delete User';
        }
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
}
