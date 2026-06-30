import api from '../../modules/api.js';

export class AdminSettings {
    constructor({ toast, modal }) {
        this.toast = toast;
        this.modal = modal;
        this.container = null;
        this.settings = null;
    }

    async mount(container) {
        this.container = container;
        await this.loadSettings();
        this.render();
        this.attachEventListeners();
    }

    async loadSettings() {
        try {
            const response = await api.get('/api/v1/admin/settings');
            this.settings = response || {};
        } catch (error) {
            console.error('Failed to load settings:', error);
            this.settings = {};
        }
    }

    render() {
        if (!this.container) return;

        this.container.innerHTML = `
            <h2 class="section-title">System Settings</h2>
            
            <div class="settings-section">
                <h3>🔐 API Configuration</h3>
                <form id="api-settings-form" class="form-stack">
                    <div class="form-group">
                        <label for="proxy-url">Proxy URL</label>
                        <input 
                            type="text" 
                            id="proxy-url" 
                            name="proxy_url" 
                            class="form-input"
                            placeholder="username:password@host:port"
                            value="${this.settings.proxy_url || ''}"
                        />
                        <small class="form-hint">Leave empty to use direct connection</small>
                    </div>
                    <div class="form-group">
                        <label for="api-key">CebuPacific API Key</label>
                        <input 
                            type="password" 
                            id="api-key" 
                            name="api_key" 
                            class="form-input"
                            placeholder="Enter API key"
                            value="${this.settings.api_key || ''}"
                        />
                    </div>
                    <div class="form-actions">
                        <button type="submit" class="btn btn-primary">Save API Settings</button>
                    </div>
                </form>
            </div>

            <div class="settings-section">
                <h3>📱 Telegram Bot Configuration</h3>
                <form id="telegram-settings-form" class="form-stack">
                    <div class="form-group">
                        <label for="bot-token">Bot Token</label>
                        <input 
                            type="password" 
                            id="bot-token" 
                            name="telegram_bot_token" 
                            class="form-input"
                            placeholder="1234567890:ABCdefGHIjklMNOpqrsTUVwxyz"
                            value="${this.settings.telegram_bot_token || ''}"
                        />
                    </div>
                    <div class="form-row">
                        <div class="form-group">
                            <label for="notification-channel">Notification Channel ID</label>
                            <input 
                                type="text" 
                                id="notification-channel" 
                                name="notification_channel" 
                                class="form-input"
                                placeholder="-1001234567890"
                                value="${this.settings.notification_channel || ''}"
                            />
                        </div>
                        <div class="form-group">
                            <label for="approval-channel">Approval Channel ID</label>
                            <input 
                                type="text" 
                                id="approval-channel" 
                                name="approval_channel" 
                                class="form-input"
                                placeholder="-1001234567891"
                                value="${this.settings.approval_channel || ''}"
                            />
                        </div>
                    </div>
                    <div class="form-actions">
                        <button type="submit" class="btn btn-primary">Save Telegram Settings</button>
                    </div>
                </form>
            </div>

            <div class="settings-section">
                <h3>💳 Payment QR Code</h3>
                <form id="qr-upload-form">
                    <div class="file-upload-area" id="qr-upload-area">
                        ${this.settings.qr_code_image ? `
                            <img src="${this.settings.qr_code_image}" alt="Payment QR Code" class="file-preview">
                            <p>Click to change QR code</p>
                        ` : `
                            <div class="empty-state-icon">📷</div>
                            <p>Click or drag to upload QR code</p>
                            <small>PNG, JPG up to 5MB</small>
                        `}
                    </div>
                    <input type="file" id="qr-file-input" accept="image/*" style="display: none;">
                    <div class="form-group">
                        <label for="payment-instructions">Payment Instructions</label>
                        <textarea 
                            id="payment-instructions" 
                            name="payment_instructions" 
                            class="form-input"
                            rows="4"
                            placeholder="Enter payment instructions for users..."
                        >${this.settings.payment_instructions || ''}</textarea>
                    </div>
                    <div class="form-actions">
                        <button type="submit" class="btn btn-primary">Update Payment Info</button>
                    </div>
                </form>
            </div>
        `;
    }

    attachEventListeners() {
        // API Settings Form
        const apiForm = this.container.querySelector('#api-settings-form');
        if (apiForm) {
            apiForm.addEventListener('submit', async (e) => {
                e.preventDefault();
                await this.saveAPISettings(apiForm);
            });
        }

        // Telegram Settings Form
        const telegramForm = this.container.querySelector('#telegram-settings-form');
        if (telegramForm) {
            telegramForm.addEventListener('submit', async (e) => {
                e.preventDefault();
                await this.saveTelegramSettings(telegramForm);
            });
        }

        // QR Upload
        const uploadArea = this.container.querySelector('#qr-upload-area');
        const fileInput = this.container.querySelector('#qr-file-input');
        if (uploadArea && fileInput) {
            uploadArea.addEventListener('click', () => fileInput.click());
            fileInput.addEventListener('change', (e) => {
                const file = e.target.files[0];
                if (file) {
                    this.previewQRCode(file);
                }
            });
        }

        // QR Form
        const qrForm = this.container.querySelector('#qr-upload-form');
        if (qrForm) {
            qrForm.addEventListener('submit', async (e) => {
                e.preventDefault();
                await this.saveQRSettings(qrForm);
            });
        }
    }

    async saveAPISettings(form) {
        const submitBtn = form.querySelector('button[type="submit"]');
        submitBtn.disabled = true;
        submitBtn.textContent = 'Saving...';

        try {
            await api.put('/api/v1/admin/settings', {
                proxy_url: form.querySelector('#proxy-url').value,
                api_key: form.querySelector('#api-key').value,
            });

            this.toast.show({
                title: 'Success',
                message: 'API settings saved successfully',
                type: 'success',
            });
        } catch (error) {
            console.error('Failed to save settings:', error);
            this.toast.show({
                title: 'Error',
                message: error.message || 'Failed to save settings',
                type: 'error',
            });
        } finally {
            submitBtn.disabled = false;
            submitBtn.textContent = 'Save API Settings';
        }
    }

    async saveTelegramSettings(form) {
        const submitBtn = form.querySelector('button[type="submit"]');
        submitBtn.disabled = true;
        submitBtn.textContent = 'Saving...';

        try {
            await api.put('/api/v1/admin/settings', {
                telegram_bot_token: form.querySelector('#bot-token').value,
                notification_channel: form.querySelector('#notification-channel').value,
                approval_channel: form.querySelector('#approval-channel').value,
            });

            this.toast.show({
                title: 'Success',
                message: 'Telegram settings saved successfully. Bot will restart automatically.',
                type: 'success',
            });
        } catch (error) {
            console.error('Failed to save settings:', error);
            this.toast.show({
                title: 'Error',
                message: error.message || 'Failed to save settings',
                type: 'error',
            });
        } finally {
            submitBtn.disabled = false;
            submitBtn.textContent = 'Save Telegram Settings';
        }
    }

    previewQRCode(file) {
        const reader = new FileReader();
        reader.onload = (e) => {
            const uploadArea = this.container.querySelector('#qr-upload-area');
            uploadArea.innerHTML = `
                <img src="${e.target.result}" alt="Payment QR Code" class="file-preview">
                <p>Click to change QR code</p>
            `;
            uploadArea.classList.add('has-file');
        };
        reader.readAsDataURL(file);
    }

    async saveQRSettings(form) {
        const submitBtn = form.querySelector('button[type="submit"]');
        const fileInput = form.querySelector('#qr-file-input');
        const instructions = form.querySelector('#payment-instructions').value;
        
        submitBtn.disabled = true;
        submitBtn.textContent = 'Uploading...';

        try {
            // Upload QR code if file selected
            if (fileInput.files.length > 0) {
                const formData = new FormData();
                formData.append('qr_code', fileInput.files[0]);

                await api.post('/api/v1/admin/settings/qr', formData, {
                    headers: { 'Content-Type': 'multipart/form-data' },
                });
            }

            // Update instructions
            await api.put('/api/v1/admin/settings', {
                payment_instructions: instructions,
            });

            this.toast.show({
                title: 'Success',
                message: 'Payment settings saved successfully',
                type: 'success',
            });

            await this.loadSettings();
            this.render();
            this.attachEventListeners();
        } catch (error) {
            console.error('Failed to save settings:', error);
            this.toast.show({
                title: 'Error',
                message: error.message || 'Failed to save settings',
                type: 'error',
            });
        } finally {
            submitBtn.disabled = false;
            submitBtn.textContent = 'Update Payment Info';
        }
    }
}
