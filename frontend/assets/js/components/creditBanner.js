import state from '../modules/state.js';

export class CreditBanner {
    constructor({ toast }) {
        this.toast = toast;
        this.container = null;
        this.unsubscribe = null;
    }

    mount(container) {
        this.container = container;
        this.render();
        
        // Subscribe to user state changes
        this.unsubscribe = state.subscribe(
            (userState) => {
                this.updateBanner(userState.profile?.credits || 0);
            },
            (currentState) => currentState.user
        );
    }

    unmount() {
        if (this.unsubscribe) {
            this.unsubscribe();
        }
        if (this.container) {
            this.container.innerHTML = '';
        }
    }

    render() {
        const profile = state.getState().user.profile || {};
        const credits = profile.credits || 0;
        
        if (credits === 0) {
            this.container.innerHTML = `
                <div class="alert alert-warning credit-banner" role="alert" id="credit-banner">
                    <div class="alert-icon">⚠️</div>
                    <div class="alert-content">
                        <h4 class="alert-title">No Credits Available</h4>
                        <p class="alert-message">
                            You need to top up credits to use the payment processor. 
                            ${profile.telegram_linked ? 
                                'Use the Telegram bot command <code>/topup [amount]</code> to request a top-up.' :
                                'Please link your Telegram account first to manage top-ups.'
                            }
                        </p>
                    </div>
                    <button class="alert-close" aria-label="Dismiss" id="dismiss-credit-banner">×</button>
                </div>
            `;
            
            const dismissBtn = this.container.querySelector('#dismiss-credit-banner');
            if (dismissBtn) {
                dismissBtn.addEventListener('click', () => {
                    this.container.querySelector('#credit-banner').remove();
                });
            }
        } else {
            this.container.innerHTML = '';
        }
    }

    updateBanner(credits) {
        this.render();
        
        // Also show modal if credits just reached 0 during a session
        if (credits === 0) {
            this.showZeroCreditModal();
        }
    }

    showZeroCreditModal() {
        const profile = state.getState().user.profile || {};
        
        // Only show once per session
        if (sessionStorage.getItem('zero_credit_modal_shown')) {
            return;
        }
        sessionStorage.setItem('zero_credit_modal_shown', 'true');
        
        this.toast.show({
            title: '💰 Credits Depleted',
            message: profile.telegram_linked ?
                'Your credits have been depleted. Please top up using the Telegram bot to continue.' :
                'Your credits have been depleted. Please link your Telegram account and top up to continue.',
            type: 'warning',
            duration: 10000,
        });
    }

    disablePaymentUI() {
        // Disable payment form inputs
        const paymentForm = document.querySelector('#payment-form');
        if (paymentForm) {
            const inputs = paymentForm.querySelectorAll('input, textarea, select, button[type="submit"]');
            inputs.forEach(input => {
                input.disabled = true;
                input.classList.add('disabled');
            });
            
            // Add overlay
            const overlay = document.createElement('div');
            overlay.className = 'form-overlay';
            overlay.innerHTML = `
                <div class="overlay-content">
                    <h3>⚠️ Credits Required</h3>
                    <p>Please top-up credits to use this service</p>
                </div>
            `;
            paymentForm.style.position = 'relative';
            paymentForm.appendChild(overlay);
        }
    }

    enablePaymentUI() {
        // Enable payment form inputs
        const paymentForm = document.querySelector('#payment-form');
        if (paymentForm) {
            const inputs = paymentForm.querySelectorAll('input, textarea, select, button[type="submit"]');
            inputs.forEach(input => {
                input.disabled = false;
                input.classList.remove('disabled');
            });
            
            // Remove overlay
            const overlay = paymentForm.querySelector('.form-overlay');
            if (overlay) {
                overlay.remove();
            }
        }
    }
}
