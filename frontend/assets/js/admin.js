import storage from './modules/storage.js';
import authService from './services/authService.js';
import { ThemeToggle } from './components/themeToggle.js';
import { ToastManager } from './components/toast.js';
import { ModalManager } from './components/modal.js';
import { AdminOverview } from './components/admin/overview.js';
import { AdminUsers } from './components/admin/users.js';
import { AdminLicenses } from './components/admin/licenses.js';
import { AdminTopups } from './components/admin/topups.js';
import { AdminSettings } from './components/admin/settings.js';

class AdminDashboard {
    constructor() {
        this.toast = null;
        this.modal = null;
        this.currentSection = 'overview';
        this.components = {};
    }

    async bootstrap() {
        // Check if user is authenticated
        if (!authService.isAuthenticated()) {
            window.location.href = '/';
            return;
        }

        // Initialize UI components
        this.toast = new ToastManager();
        this.toast.mount(document.getElementById('toast-root'));

        this.modal = new ModalManager();
        this.modal.mount(document.getElementById('modal-root'));

        const themeToggle = new ThemeToggle();
        themeToggle.mount(document.getElementById('theme-toggle'));

        // Initialize admin sections
        this.components = {
            overview: new AdminOverview({ toast: this.toast, modal: this.modal }),
            users: new AdminUsers({ toast: this.toast, modal: this.modal }),
            licenses: new AdminLicenses({ toast: this.toast, modal: this.modal }),
            topups: new AdminTopups({ toast: this.toast, modal: this.modal }),
            settings: new AdminSettings({ toast: this.toast, modal: this.modal }),
        };

        // Setup navigation
        this.setupNavigation();
        
        // Setup logout
        this.setupLogout();

        // Show initial section
        this.showSection('overview');
    }

    setupNavigation() {
        const navItems = document.querySelectorAll('.nav-item');
        navItems.forEach(item => {
            item.addEventListener('click', (e) => {
                e.preventDefault();
                const section = item.dataset.section;
                this.showSection(section);
                
                // Update active state
                navItems.forEach(nav => nav.classList.remove('active'));
                item.classList.add('active');
            });
        });
    }

    setupLogout() {
        const logoutBtn = document.getElementById('logout-btn');
        if (logoutBtn) {
            logoutBtn.addEventListener('click', async () => {
                await authService.logout();
                window.location.href = '/';
            });
        }
    }

    showSection(sectionName) {
        this.currentSection = sectionName;
        const container = document.getElementById('admin-main-mount');
        if (!container) return;

        // Unmount current component
        container.innerHTML = '';

        // Mount new component
        const component = this.components[sectionName];
        if (component) {
            component.mount(container);
        }
    }
}

// Bootstrap admin dashboard
if (typeof document !== 'undefined') {
    document.addEventListener('DOMContentLoaded', async () => {
        const dashboard = new AdminDashboard();
        await dashboard.bootstrap();
    });
}
