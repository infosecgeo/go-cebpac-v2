import authService from '../services/authService.js';
import state from '../modules/state.js';

export class LoginForm {
    constructor({ toast, modal }) {
        this.toast = toast;
        this.modal = modal;
        this.container = null;
    }

    mount(container) {
        this.container = container;
        this.render();
        this.attachEventListeners();
    }

    render() {
        this.container.innerHTML = `
            <div class="shell-card">
                <div class="card-header">
                    <h2>🔐 License Key Login</h2>
                    <p class="card-subtitle">Enter your license key to access the payment processor</p>
                </div>
                <div class="card-body">
                    <form id="login-form" class="form-stack">
                        <div class="form-group">
                            <label for="license-key">License Key</label>
                            <input 
                                type="text" 
                                id="license-key" 
                                name="license_key" 
                                placeholder="XXXXX-XXXXX-XXXXX-XXXXX"
                                class="form-input"
                                required
                                autocomplete="off"
                            />
                            <small class="form-hint">Format: XXXXX-XXXXX-XXXXX-XXXXX</small>
                        </div>
                        <div class="form-actions">
                            <button type="submit" class="btn btn-primary btn-block" id="login-btn">
                                <span class="btn-text">Login</span>
                                <span class="btn-loader" style="display: none;">Processing...</span>
                            </button>
                        </div>
                    </form>
                    <div class="card-footer">
                        <p class="text-center">
                            Don't have a license? 
                            <a href="#" id="show-register-link" class="link">Register here</a>
                        </p>
                        <p class="text-center">
                            <a href="#" id="show-admin-login-link" class="link">Admin Login</a>
                        </p>
                    </div>
                </div>
            </div>
        `;
    }

    attachEventListeners() {
        const form = this.container.querySelector('#login-form');
        const loginBtn = this.container.querySelector('#login-btn');
        const showRegisterLink = this.container.querySelector('#show-register-link');
        const showAdminLoginLink = this.container.querySelector('#show-admin-login-link');

        form.addEventListener('submit', async (e) => {
            e.preventDefault();
            await this.handleLogin();
        });

        showRegisterLink.addEventListener('click', (e) => {
            e.preventDefault();
            this.showRegistrationForm();
        });

        showAdminLoginLink.addEventListener('click', (e) => {
            e.preventDefault();
            this.showAdminLoginForm();
        });
    }

    async handleLogin() {
        const form = this.container.querySelector('#login-form');
        const loginBtn = this.container.querySelector('#login-btn');
        const btnText = loginBtn.querySelector('.btn-text');
        const btnLoader = loginBtn.querySelector('.btn-loader');
        const licenseKeyInput = form.querySelector('#license-key');
        const licenseKey = licenseKeyInput.value.trim().toUpperCase();

        if (!licenseKey) {
            this.toast.show({
                title: 'Validation Error',
                message: 'Please enter your license key',
                type: 'error',
            });
            return;
        }

        // Disable button
        loginBtn.disabled = true;
        btnText.style.display = 'none';
        btnLoader.style.display = 'inline';

        try {
            await authService.login(licenseKey);
            this.toast.show({
                title: 'Success',
                message: 'Login successful!',
                type: 'success',
            });
            // Reload page to show main interface
            setTimeout(() => window.location.reload(), 500);
        } catch (error) {
            console.error('Login failed:', error);
            let message = 'Login failed. Please check your license key.';
            
            if (error.message && error.message.includes('Terms and conditions')) {
                message = 'Please accept terms and conditions during registration.';
            } else if (error.message && error.message.includes('not active')) {
                message = 'Your account is not active. Please top up credits to activate.';
            }
            
            this.toast.show({
                title: 'Login Failed',
                message,
                type: 'error',
            });
        } finally {
            loginBtn.disabled = false;
            btnText.style.display = 'inline';
            btnLoader.style.display = 'none';
        }
    }

    showRegistrationForm() {
        this.modal.show({
            title: 'Register New License',
            content: `
                <form id="register-form" class="form-stack">
                    <div class="form-group">
                        <label for="reg-license-key">License Key</label>
                        <input 
                            type="text" 
                            id="reg-license-key" 
                            name="license_key" 
                            placeholder="XXXXX-XXXXX-XXXXX-XXXXX"
                            class="form-input"
                            required
                        />
                    </div>
                    <div class="form-group">
                        <label class="checkbox-label">
                            <input type="checkbox" id="terms-checkbox" name="terms_accepted" required />
                            <span>I accept the <a href="#" id="view-terms-link">Terms and Conditions</a></span>
                        </label>
                    </div>
                    <div class="alert alert-info">
                        <strong>⚠️ Important:</strong> You must top up at least 1 credit to activate your account.
                        After registration, link your Telegram account for top-up management.
                    </div>
                    <div class="form-actions">
                        <button type="button" class="btn btn-secondary" id="cancel-register-btn">Cancel</button>
                        <button type="submit" class="btn btn-primary" id="register-btn">Register</button>
                    </div>
                </form>
            `,
            onMount: (modalElement) => {
                const form = modalElement.querySelector('#register-form');
                const registerBtn = modalElement.querySelector('#register-btn');
                const cancelBtn = modalElement.querySelector('#cancel-register-btn');
                const viewTermsLink = modalElement.querySelector('#view-terms-link');

                form.addEventListener('submit', async (e) => {
                    e.preventDefault();
                    await this.handleRegistration(form, registerBtn);
                });

                cancelBtn.addEventListener('click', () => {
                    this.modal.hide();
                });

                viewTermsLink.addEventListener('click', (e) => {
                    e.preventDefault();
                    this.showTermsAndConditions();
                });
            },
        });
    }

    async handleRegistration(form, registerBtn) {
        const licenseKey = form.querySelector('#reg-license-key').value.trim().toUpperCase();
        const termsAccepted = form.querySelector('#terms-checkbox').checked;

        if (!licenseKey || !termsAccepted) {
            this.toast.show({
                title: 'Validation Error',
                message: 'Please fill all fields and accept terms',
                type: 'error',
            });
            return;
        }

        registerBtn.disabled = true;
        registerBtn.textContent = 'Registering...';

        try {
            const result = await authService.register(licenseKey, termsAccepted);
            this.modal.hide();
            this.toast.show({
                title: 'Registration Successful',
                message: result.message || 'Please top up at least 1 credit to activate your account.',
                type: 'success',
                duration: 8000,
            });
            
            // Show Telegram linking instructions
            if (result.telegram_bot) {
                setTimeout(() => {
                    this.showTelegramInstructions(result.telegram_bot);
                }, 1000);
            }
        } catch (error) {
            console.error('Registration failed:', error);
            this.toast.show({
                title: 'Registration Failed',
                message: error.message || 'Failed to register. Please try again.',
                type: 'error',
            });
        } finally {
            registerBtn.disabled = false;
            registerBtn.textContent = 'Register';
        }
    }

    showTermsAndConditions() {
        // Create a nested modal for T&C
        const termsModal = document.createElement('div');
        termsModal.className = 'modal-overlay';
        termsModal.innerHTML = `
            <div class="modal-dialog" style="max-width: 600px;">
                <div class="modal-header">
                    <h3>Terms and Conditions</h3>
                </div>
                <div class="modal-body" style="max-height: 400px; overflow-y: auto;">
                    <h4>1. License Usage</h4>
                    <p>Each license is for single-user use only. One license per Telegram account.</p>
                    
                    <h4>2. Credits and Payment</h4>
                    <p>You must maintain at least 1 credit to use the service. Credits are deducted per successful transaction.</p>
                    
                    <h4>3. Account Security</h4>
                    <p>You are responsible for maintaining the security of your license key. Do not share your license.</p>
                    
                    <h4>4. Dual Login Prevention</h4>
                    <p>Only one active session is allowed per license. Logging in from a new device will terminate previous sessions.</p>
                    
                    <h4>5. Service Availability</h4>
                    <p>We strive for 99% uptime but do not guarantee uninterrupted service.</p>
                    
                    <h4>6. Refund Policy</h4>
                    <p>Credits are non-refundable once purchased and approved.</p>
                    
                    <h4>7. Termination</h4>
                    <p>We reserve the right to suspend or terminate accounts that violate these terms.</p>
                </div>
                <div class="modal-footer">
                    <button type="button" class="btn btn-primary" id="close-terms-btn">Close</button>
                </div>
            </div>
        `;
        document.body.appendChild(termsModal);
        
        const closeBtn = termsModal.querySelector('#close-terms-btn');
        closeBtn.addEventListener('click', () => {
            termsModal.remove();
        });
        
        termsModal.addEventListener('click', (e) => {
            if (e.target === termsModal) {
                termsModal.remove();
            }
        });
    }

    showTelegramInstructions(botCommand) {
        this.modal.show({
            title: '📱 Link Your Telegram Account',
            content: `
                <div class="alert alert-info">
                    <h4>Next Steps:</h4>
                    <ol>
                        <li>Open Telegram on your device</li>
                        <li>${botCommand}</li>
                        <li>Use the <code>/topup</code> command to request credit top-up</li>
                        <li>Upload payment receipt when prompted</li>
                        <li>Wait for admin approval</li>
                    </ol>
                    <p><strong>Once approved, you can login with your license key!</strong></p>
                </div>
                <div class="form-actions">
                    <button type="button" class="btn btn-primary btn-block" id="got-it-btn">Got it!</button>
                </div>
            `,
            onMount: (modalElement) => {
                const gotItBtn = modalElement.querySelector('#got-it-btn');
                gotItBtn.addEventListener('click', () => {
                    this.modal.hide();
                });
            },
        });
    }

    showAdminLoginForm() {
        this.modal.show({
            title: '👨‍💼 Admin Login',
            content: `
                <form id="admin-login-form" class="form-stack">
                    <div class="form-group">
                        <label for="admin-username">Username</label>
                        <input 
                            type="text" 
                            id="admin-username" 
                            name="username" 
                            class="form-input"
                            required
                            autocomplete="username"
                        />
                    </div>
                    <div class="form-group">
                        <label for="admin-password">Password</label>
                        <input 
                            type="password" 
                            id="admin-password" 
                            name="password" 
                            class="form-input"
                            required
                            autocomplete="current-password"
                        />
                    </div>
                    <div class="form-actions">
                        <button type="button" class="btn btn-secondary" id="cancel-admin-btn">Cancel</button>
                        <button type="submit" class="btn btn-primary" id="admin-login-btn">Login</button>
                    </div>
                </form>
            `,
            onMount: (modalElement) => {
                const form = modalElement.querySelector('#admin-login-form');
                const adminLoginBtn = modalElement.querySelector('#admin-login-btn');
                const cancelBtn = modalElement.querySelector('#cancel-admin-btn');

                form.addEventListener('submit', async (e) => {
                    e.preventDefault();
                    await this.handleAdminLogin(form, adminLoginBtn);
                });

                cancelBtn.addEventListener('click', () => {
                    this.modal.hide();
                });
            },
        });
    }

    async handleAdminLogin(form, adminLoginBtn) {
        const username = form.querySelector('#admin-username').value.trim();
        const password = form.querySelector('#admin-password').value;

        if (!username || !password) {
            this.toast.show({
                title: 'Validation Error',
                message: 'Please enter username and password',
                type: 'error',
            });
            return;
        }

        adminLoginBtn.disabled = true;
        adminLoginBtn.textContent = 'Logging in...';

        try {
            await authService.adminLogin(username, password);
            this.modal.hide();
            this.toast.show({
                title: 'Admin Login Successful',
                message: 'Redirecting to admin dashboard...',
                type: 'success',
            });
            // Redirect to admin dashboard
            setTimeout(() => window.location.href = '/admin.html', 500);
        } catch (error) {
            console.error('Admin login failed:', error);
            this.toast.show({
                title: 'Admin Login Failed',
                message: 'Invalid credentials. Please try again.',
                type: 'error',
            });
        } finally {
            adminLoginBtn.disabled = false;
            adminLoginBtn.textContent = 'Login';
        }
    }
}
