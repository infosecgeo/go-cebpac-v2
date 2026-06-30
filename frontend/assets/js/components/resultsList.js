import state from '../modules/state.js';
        import { copyToClipboard, downloadTextFile, toCsv } from '../utils/helpers.js';
        import { formatDuration, formatTimestamp, maskCard } from '../utils/formatters.js';

        export class ResultsList {
            constructor({ toast, modal }) {
                this.toast = toast;
                this.modal = modal;
                this.container = null;
                this.filter = 'all';
                this.liveList = null;
                this.deadList = null;
            }

            mount(container) {
                this.container = container;
                this.container.innerHTML = `
                    <section class="card stack-md" aria-labelledby="results-title">
                        <div class="results-toolbar">
                            <div>
                                <h2 class="card__title" id="results-title">Payment results</h2>
                                <p class="card__subtitle">Separate live and dead outcomes, copy records instantly, and export summaries.</p>
                            </div>
                            <div class="toolbar-group">
                                <select class="select" id="results-filter" aria-label="Filter results">
                                    <option value="all">All results</option>
                                    <option value="live">Live only</option>
                                    <option value="dead">Dead only</option>
                                </select>
                                <button type="button" class="btn btn-secondary" id="results-copy">Copy visible</button>
                                <button type="button" class="btn btn-secondary" id="results-export">Export CSV</button>
                                <button type="button" class="btn btn-ghost" id="results-clear">Clear</button>
                            </div>
                        </div>
                        <div class="results-grid">
                            <section class="results-column" aria-labelledby="live-results-title">
                                <div class="metric-inline"><h3 class="card__title" id="live-results-title">Live</h3><span class="badge badge--success" id="live-count">0</span></div>
                                <div class="result-list" id="live-results-list"></div>
                            </section>
                            <section class="results-column" aria-labelledby="dead-results-title">
                                <div class="metric-inline"><h3 class="card__title" id="dead-results-title">Dead</h3><span class="badge badge--danger" id="dead-count">0</span></div>
                                <div class="result-list" id="dead-results-list"></div>
                            </section>
                        </div>
                    </section>
                `;
                this.liveList = this.container.querySelector('#live-results-list');
                this.deadList = this.container.querySelector('#dead-results-list');
                this.container.querySelector('#results-filter').addEventListener('change', (event) => {
                    this.filter = event.target.value;
                    this.render(state.getState());
                });
                this.container.querySelector('#results-copy').addEventListener('click', () => this.copyVisible());
                this.container.querySelector('#results-export').addEventListener('click', () => this.exportResults());
                this.container.querySelector('#results-clear').addEventListener('click', async () => {
                    const confirmed = await this.modal.confirm({
                        title: 'Clear stored results?',
                        description: 'This removes the visible live and dead result history from the page.',
                        confirmText: 'Clear results',
                    });
                    if (confirmed) {
                        state.clearResults();
                        this.toast.show({ title: 'Results cleared', message: 'Result history was removed from the dashboard.', type: 'info' });
                    }
                });
                this.render(state.getState());
                state.subscribe((nextState) => this.render(nextState), (nextState) => ({ results: nextState.results, running: nextState.processing.running }));
            }

            getVisibleResults(results) {
                if (this.filter === 'all') {
                    return results;
                }
                return results.filter((result) => result.status === this.filter);
            }

            createResultItem(result) {
                const wrapper = document.createElement('article');
                wrapper.className = 'result-item';
                const header = document.createElement('div');
                header.className = 'result-item__header';
                const badge = document.createElement('span');
                badge.className = `badge ${result.status === 'live' ? 'badge--success' : 'badge--danger'}`;
                badge.textContent = result.status.toUpperCase();
                const copyButton = document.createElement('button');
                copyButton.type = 'button';
                copyButton.className = 'btn btn-ghost btn-icon';
                copyButton.textContent = 'Copy';
                copyButton.addEventListener('click', async () => {
                    await copyToClipboard(`${result.card}
${result.message}`);
                    this.toast.show({ title: 'Copied', message: 'Result copied to your clipboard.', type: 'success', duration: 2500 });
                });
                header.append(badge, copyButton);

                const body = document.createElement('div');
                body.className = 'stack-xs';
                const card = document.createElement('strong');
                card.className = 'mono';
                card.textContent = maskCard(result.card);
                const message = document.createElement('p');
                message.className = 'result-item__message';
                message.textContent = result.message;
                body.append(card, message);

                const footer = document.createElement('div');
                footer.className = 'result-item__footer result-item__meta';
                const left = document.createElement('span');
                left.textContent = `Attempt ${result.attempt} • ${formatDuration(result.durationMs)}`;
                const right = document.createElement('span');
                right.textContent = formatTimestamp(result.timestamp);
                footer.append(left, right);

                wrapper.append(header, body, footer);
                return wrapper;
            }

            renderColumn(listElement, items, emptyMessage) {
                listElement.innerHTML = '';
                if (!items.length) {
                    const empty = document.createElement('div');
                    empty.className = 'empty-state';
                    empty.innerHTML = `<strong>No results</strong><span>${emptyMessage}</span>`;
                    listElement.appendChild(empty);
                    return;
                }
                items.forEach((item) => listElement.appendChild(this.createResultItem(item)));
            }

            render(nextState) {
                const results = nextState.results.slice().reverse();
                const visibleResults = this.getVisibleResults(results);
                const liveResults = visibleResults.filter((result) => result.status === 'live');
                const deadResults = visibleResults.filter((result) => result.status === 'dead');
                this.container.querySelector('#live-count').textContent = String(liveResults.length);
                this.container.querySelector('#dead-count').textContent = String(deadResults.length);
                this.renderColumn(this.liveList, liveResults, 'Live results appear here when the backend approves a payment.');
                this.renderColumn(this.deadList, deadResults, 'Declines and validation failures appear here in real time.');
            }

            async copyVisible() {
                const visible = this.getVisibleResults(state.getState().results);
                if (!visible.length) {
                    this.toast.show({ title: 'Nothing to copy', message: 'Run a payment first, then copy the visible results.', type: 'warning' });
                    return;
                }
                const content = visible.map((item) => `${item.status.toUpperCase()} | ${item.card} | ${item.message}`).join('\n');
                await copyToClipboard(content);
                this.toast.show({ title: 'Visible results copied', message: `${visible.length} result(s) copied to the clipboard.`, type: 'success' });
            }

            exportResults() {
                const results = state.getState().results;
                if (!results.length) {
                    this.toast.show({ title: 'Nothing to export', message: 'There are no results available yet.', type: 'warning' });
                    return;
                }
                const csv = toCsv([
                    ['status', 'card', 'message', 'attempt', 'duration_ms', 'timestamp'],
                    ...results.map((item) => [item.status, item.card, item.message, item.attempt, Math.round(item.durationMs), item.timestamp]),
                ]);
                downloadTextFile(`cebpac-results-${Date.now()}.csv`, csv, 'text/csv;charset=utf-8');
                this.toast.show({ title: 'Export started', message: 'The current results set was exported as CSV.', type: 'info' });
            }
        }
