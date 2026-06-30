export class ProgressLogger {
    constructor() {
        this.container = null;
        this.logs = [];
    }

    mount(container) {
        this.container = container;
        this.render();
    }

    render() {
        if (!this.container) return;
        
        this.container.innerHTML = `
            <div class="progress-logger">
                <div class="progress-header">
                    <h4>Processing Status</h4>
                    <button class="btn-icon" id="clear-logs" title="Clear logs">🗑️</button>
                </div>
                <div class="progress-bar-container">
                    <div class="progress-bar">
                        <div class="progress-fill" id="progress-fill" style="width: 0%"></div>
                    </div>
                    <div class="progress-text" id="progress-text">0%</div>
                </div>
                <div class="progress-step" id="progress-step"></div>
                <div class="log-panel" id="log-panel">
                    <div class="log-entries" id="log-entries"></div>
                </div>
            </div>
        `;
        
        const clearBtn = this.container.querySelector('#clear-logs');
        if (clearBtn) {
            clearBtn.addEventListener('click', () => this.clearLogs());
        }
    }

    updateProgress(percent, step = null) {
        const progressFill = this.container?.querySelector('#progress-fill');
        const progressText = this.container?.querySelector('#progress-text');
        const progressStep = this.container?.querySelector('#progress-step');
        
        if (progressFill) {
            progressFill.style.width = `${percent}%`;
        }
        if (progressText) {
            progressText.textContent = `${percent}%`;
        }
        if (progressStep && step) {
            progressStep.textContent = step;
        }
    }

    addLog(message, type = 'info') {
        const timestamp = new Date().toLocaleTimeString('en-US', { 
            hour12: false,
            hour: '2-digit',
            minute: '2-digit',
            second: '2-digit' 
        });
        
        const log = {
            timestamp,
            message,
            type, // info, success, error, warning
        };
        
        this.logs.push(log);
        
        const logEntries = this.container?.querySelector('#log-entries');
        if (logEntries) {
            const logEntry = document.createElement('div');
            logEntry.className = `log-entry log-${type}`;
            logEntry.innerHTML = `
                <span class="log-time">[${timestamp}]</span>
                <span class="log-icon">${this.getLogIcon(type)}</span>
                <span class="log-message">${this.escapeHtml(message)}</span>
            `;
            logEntries.appendChild(logEntry);
            
            // Auto-scroll to bottom
            logEntries.scrollTop = logEntries.scrollHeight;
        }
    }

    getLogIcon(type) {
        const icons = {
            info: 'ℹ️',
            success: '✅',
            error: '❌',
            warning: '⚠️',
        };
        return icons[type] || 'ℹ️';
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    clearLogs() {
        this.logs = [];
        const logEntries = this.container?.querySelector('#log-entries');
        if (logEntries) {
            logEntries.innerHTML = '';
        }
        this.updateProgress(0, '');
    }

    reset() {
        this.clearLogs();
    }
}
