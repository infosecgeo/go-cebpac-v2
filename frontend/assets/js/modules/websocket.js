import state from './state.js';

        export class WebSocketClient extends EventTarget {
            constructor({ url, reconnect = true, heartbeatInterval = 15000, maxReconnectDelay = 30000 } = {}) {
                super();
                this.url = url;
                this.reconnect = reconnect;
                this.heartbeatInterval = heartbeatInterval;
                this.maxReconnectDelay = maxReconnectDelay;
                this.socket = null;
                this.reconnectAttempts = 0;
                this.heartbeatTimer = null;
                this.pingSentAt = null;
                this.shouldReconnect = reconnect;
            }

            connect() {
                if (!this.url || this.socket?.readyState === WebSocket.OPEN || this.socket?.readyState === WebSocket.CONNECTING) {
                    return;
                }

                this.updateStatus('connecting');
                this.socket = new WebSocket(this.url);

                this.socket.addEventListener('open', () => {
                    this.reconnectAttempts = 0;
                    this.updateStatus('connected');
                    this.startHeartbeat();
                    this.dispatchEvent(new CustomEvent('open'));
                });

                this.socket.addEventListener('message', (event) => this.handleMessage(event.data));
                this.socket.addEventListener('close', () => {
                    this.stopHeartbeat();
                    const nextStatus = this.shouldReconnect ? 'reconnecting' : 'disconnected';
                    this.updateStatus(nextStatus);
                    this.dispatchEvent(new CustomEvent('close'));
                    if (this.shouldReconnect) {
                        this.scheduleReconnect();
                    }
                });
                this.socket.addEventListener('error', () => {
                    this.updateStatus('error');
                    this.dispatchEvent(new CustomEvent('error'));
                });
            }

            disconnect() {
                this.shouldReconnect = false;
                this.stopHeartbeat();
                this.socket?.close();
                this.updateStatus('disconnected');
            }

            scheduleReconnect() {
                this.reconnectAttempts += 1;
                const delay = Math.min(1000 * 2 ** (this.reconnectAttempts - 1), this.maxReconnectDelay);
                window.setTimeout(() => {
                    if (this.shouldReconnect) {
                        this.connect();
                    }
                }, delay);
            }

            startHeartbeat() {
                this.stopHeartbeat();
                this.heartbeatTimer = window.setInterval(() => {
                    if (this.socket?.readyState !== WebSocket.OPEN) {
                        return;
                    }
                    this.pingSentAt = performance.now();
                    this.send('heartbeat', { timestamp: Date.now() });
                }, this.heartbeatInterval);
            }

            stopHeartbeat() {
                if (this.heartbeatTimer) {
                    window.clearInterval(this.heartbeatTimer);
                    this.heartbeatTimer = null;
                }
            }

            send(type, data = {}) {
                if (this.socket?.readyState !== WebSocket.OPEN) {
                    return false;
                }
                this.socket.send(JSON.stringify({ type, data }));
                return true;
            }

            updateStatus(status) {
                const connected = status === 'connected';
                state.setState({
                    socket: {
                        status,
                        connected,
                        reconnectAttempts: this.reconnectAttempts,
                        lastConnectedAt: connected ? new Date().toISOString() : state.getState().socket.lastConnectedAt,
                    },
                }, 'socket:status');
                this.dispatchEvent(new CustomEvent('statuschange', {
                    detail: { status, connected, reconnectAttempts: this.reconnectAttempts },
                }));
            }

            handleMessage(rawMessage) {
                const messages = String(rawMessage)
                    .split('\n')
                    .map((entry) => entry.trim())
                    .filter(Boolean);

                messages.forEach((entry) => {
                    let payload;
                    try {
                        payload = JSON.parse(entry);
                    } catch {
                        return;
                    }

                    if (payload.type === 'heartbeat' && this.pingSentAt) {
                        const latency = performance.now() - this.pingSentAt;
                        state.setState({ socket: { latencyMs: latency }, stats: { latencyMs: latency } }, 'socket:latency');
                    }

                    this.dispatchEvent(new CustomEvent('message', { detail: payload }));
                    if (payload.type) {
                        this.dispatchEvent(new CustomEvent(payload.type, { detail: payload }));
                    }
                });
            }
        }
