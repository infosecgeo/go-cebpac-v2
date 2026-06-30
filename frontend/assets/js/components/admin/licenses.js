import api from '../../modules/api.js';

export class AdminLicenses {
    constructor({ toast, modal }) {
        this.toast = toast;
        this.modal = modal;
        this.container = null;
        this.licenses = [];
    }

    async mount(container) {
        this.container = container;
        await this.loadLicenses();
        this.render();
        this.attachEventListeners();
    }

    async loadLicenses() {
        try {
            const response = await api.get('/api/v1/admin/licenses');
            this.licenses = response.licenses || [];
        } catch (error) {
            console.error('Failed to load licenses:', error);
            this.licenses = [];
        }
    }

    render() {
        if (!this.container) return;

        this.container.innerHTML = `
            <h2 class="section-title">License Management</h2>
            
            <div class="admin-table-container">
                <div class="table-header">
                    <h3 class="table-title">All Licenses</h3>
                    <div class="table-actions">
                        <button class="btn btn-primary" id="create-license-btn">
                            + Generate License
                        </button>
                    </div>
                </div>
                
                ${this.licenses.length > 0 ? `
                    <table class="data-table">
                        <thead>
                            <tr>
                                <th>License Key</th>
                                <th>Status</th>
                                <th>Max Devices</th>
                                <th>Linked Telegram</th>
                                <th>Expires</th>
                                <th>Actions</th>
                            </tr>
                        </thead>
                        <tbody>
                            ${this.licenses.map(license => `
                                <tr>
                                    <td><code>${license.license_key}</code></td>
                                    <td>
                                        <span class="status-badge status-${license.status}">
                                            ${license.status}
                                        </span>
                                    </td>
                                    <td>${license.max_devices || 1}</td>
                                    <td>
                                        ${license.linked_telegram_id ? 
                                            `ID: ${license.linked_telegram_id}` : 
                                            '<span style="opacity: 0.5;">Not linked</span>'}
                                    </td>
                                    <td>${license.expires_at ? new Date(license.expires_at).toLocaleDateString() : 'Never'}</td>
                                    <td>
                                        <div class="action-buttons">
                                            <button 
                                                class="btn btn-sm btn-danger revoke-license-btn" 
                                                data-license-id="${license.id}"
                                            >
                                                Revoke
                                            </button>
                                        </div>
                                    </td>
                                </tr>
                            `).join('')}
                        </tbody>
                    </table>
                ` : `
                    <div class="empty-state">
                        <div class="empty-state-icon">🔑</div>
                        <div class="empty-state-title">No licenses found</div>
                        <p>Generate a license to get started.</p>
                    </div>
                `}
            </div>
        `;
    }

    attachEventListeners() {
        const createBtn = this.container.querySelector('#create-license-btn');
        if (createBtn) {
            createBtn.addEventListener('click', () => this.showCreateLicenseModal());
        }

        const revokeButtons = this.container.querySelectorAll('.revoke-license-btn');
        revokeButtons.forEach(btn => {
            btn.addEventListener('click', () => {
                const licenseId = btn.dataset.licenseId;
                this.confirmRevokeLicense(licenseId);
            });
        });
    }

    showCreateLicenseModal() {
        this.modal.show({
            title: 'Generate New License',
            content: `
                <form id="create-license-form" class="form-stack">
                    <div class="form-group">
                        <label for="max-devices">Max Devices</label>
                        <input 
                            type="number" 
                            id="max-devices" 
                            name="max_devices" 
                            class="form-input"
                            value="1"
                            min="1"
                            required
                        />
                    </div>
                    <div class="form-group">
                        <label for="expiry-days">Expires In (days, 0 = never)</label>
                        <input 
                            type="number" 
                            id="expiry-days" 
                            name="expiry_days" 
                            class="form-input"
                            value="365"
                            min="0"
                        />
                    </div>
                    <div class="form-actions">
                        <button type="button" class="btn btn-secondary" id="cancel-create-btn">Cancel</button>
                        <button type="submit" class="btn btn-primary" id="generate-license-btn">Generate</button>
                    </div>
                </form>
            `,
            onMount: (modalElement) => {
                const form = modalElement.querySelector('#create-license-form');
                const generateBtn = modalElement.querySelector('#generate-license-btn');
                const cancelBtn = modalElement.querySelector('#cancel-create-btn');

                form.addEventListener('submit', async (e) => {
                    e.preventDefault();
                    await this.createLicense(form, generateBtn);
                });

                cancelBtn.addEventListener('click', () => {
                    this.modal.hide();
                });
            },
        });
    }

    async createLicense(form, generateBtn) {
        const maxDevices = parseInt(form.querySelector('#max-devices').value);
        const expiryDays = parseInt(form.querySelector('#expiry-days').value);

        generateBtn.disabled = true;
        generateBtn.textContent = 'Generating...';

        try {
            const response = await api.post('/api/v1/admin/licenses', {
                max_devices: maxDevices,
                expiry_days: expiryDays > 0 ? expiryDays : null,
            });

            this.modal.hide();
            this.toast.show({
                title: 'License Generated',
                message: `License key: ${response.license_key}`,
                type: 'success',
                duration: 10000,
            });

            await this.loadLicenses();
            this.render();
            this.attachEventListeners();
        } catch (error) {
            console.error('Failed to create license:', error);
            this.toast.show({
                title: 'Error',
                message: error.message || 'Failed to generate license',
                type: 'error',
            });
        } finally {
            generateBtn.disabled = false;
            generateBtn.textContent = 'Generate';
        }
    }

    confirmRevokeLicense(licenseId) {
        const license = this.licenses.find(l => l.id === licenseId);
        if (!license) return;

        this.modal.show({
            title: 'Confirm Revoke',
            content: `
                <div class="alert alert-warning">
                    <div class="alert-icon">⚠️</div>
                    <div class="alert-content">
                        <p>Are you sure you want to revoke license <strong>${license.license_key}</strong>?</p>
                        <p>Users with this license will lose access immediately.</p>
                    </div>
                </div>
                <div class="form-actions">
                    <button type="button" class="btn btn-secondary" id="cancel-revoke-btn">Cancel</button>
                    <button type="button" class="btn btn-danger" id="confirm-revoke-btn">Revoke License</button>
                </div>
            `,
            onMount: (modalElement) => {
                const confirmBtn = modalElement.querySelector('#confirm-revoke-btn');
                const cancelBtn = modalElement.querySelector('#cancel-revoke-btn');

                confirmBtn.addEventListener('click', async () => {
                    await this.revokeLicense(licenseId, confirmBtn);
                });

                cancelBtn.addEventListener('click', () => {
                    this.modal.hide();
                });
            },
        });
    }

    async revokeLicense(licenseId, confirmBtn) {
        confirmBtn.disabled = true;
        confirmBtn.textContent = 'Revoking...';

        try {
            await api.delete(`/api/v1/admin/licenses/${licenseId}`);

            this.modal.hide();
            this.toast.show({
                title: 'Success',
                message: 'License revoked successfully',
                type: 'success',
            });

            await this.loadLicenses();
            this.render();
            this.attachEventListeners();
        } catch (error) {
            console.error('Failed to revoke license:', error);
            this.toast.show({
                title: 'Error',
                message: error.message || 'Failed to revoke license',
                type: 'error',
            });
            confirmBtn.disabled = false;
            confirmBtn.textContent = 'Revoke License';
        }
    }
}
