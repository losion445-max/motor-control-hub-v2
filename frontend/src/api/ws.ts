import type {
  ConnectionState,
  InCommand,
  OutEvent,
  SystemStatus,
} from "./types"

class WsClient {
  private ws: WebSocket | null = null
  private url: string
  private backoff = 1000
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null
  private commandSubs = new Map<string, (e: OutEvent) => void>()
  private statusSubs = new Set<(s: SystemStatus) => void>()
  private connSubs = new Set<(s: ConnectionState) => void>()
  private state: ConnectionState = "disconnected"
  private queue: string[] = []

  constructor(url: string) {
    this.url = url
  }

  private setState(s: ConnectionState) {
    this.state = s
    this.connSubs.forEach((cb) => cb(s))
  }

  connect(url?: string) {
    if (url) this.url = url
    if (this.reconnectTimer !== null) {
      clearTimeout(this.reconnectTimer)
      this.reconnectTimer = null
    }
    if (this.ws) {
      this.ws.onclose = null
      this.ws.close()
      this.ws = null
    }
    this.setState("connecting")
    try {
      const ws = new WebSocket(this.url)
      this.ws = ws

      ws.onopen = () => {
        this.backoff = 1000
        this.setState("connected")
        const queued = this.queue.splice(0)
        queued.forEach((msg) => ws.send(msg))
      }

      ws.onmessage = (ev) => {
        let parsed: OutEvent
        try {
          parsed = JSON.parse(ev.data as string) as OutEvent
        } catch {
          return
        }
        if (parsed.kind === "status" && parsed.payload) {
          this.statusSubs.forEach((cb) => cb(parsed.payload as SystemStatus))
          return
        }
        if (parsed.id) {
          const sub = this.commandSubs.get(parsed.id)
          if (sub) {
            sub(parsed)
            if (parsed.kind === "done" || parsed.kind === "error") {
              this.commandSubs.delete(parsed.id)
            }
          }
        }
      }

      ws.onerror = () => {
        ws.close()
      }

      ws.onclose = () => {
        this.ws = null
        this.setState("disconnected")
        this.scheduleReconnect()
      }
    } catch {
      this.setState("disconnected")
      this.scheduleReconnect()
    }
  }

  private scheduleReconnect() {
    if (this.reconnectTimer !== null) return
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null
      this.connect()
    }, this.backoff)
    this.backoff = Math.min(this.backoff * 2, 30000)
  }

  send(msg: InCommand) {
    const serialized = JSON.stringify(msg)
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(serialized)
    } else {
      this.queue.push(serialized)
    }
  }

  subscribeCmd(id: string, cb: (e: OutEvent) => void): () => void {
    this.commandSubs.set(id, cb)
    return () => this.commandSubs.delete(id)
  }

  onStatus(cb: (s: SystemStatus) => void): () => void {
    this.statusSubs.add(cb)
    return () => this.statusSubs.delete(cb)
  }

  onConnection(cb: (s: ConnectionState) => void): () => void {
    this.connSubs.add(cb)
    cb(this.state)
    return () => this.connSubs.delete(cb)
  }

  getState(): ConnectionState {
    return this.state
  }

  getUrl(): string {
    return this.url
  }
}

let _client: WsClient | null = null

export function getWsClient(url?: string): WsClient {
  const defaultUrl =
    localStorage.getItem("ws_url") ?? "ws://localhost:8080/ws"
  if (!_client) {
    _client = new WsClient(url ?? defaultUrl)
    _client.connect()
  } else if (url && url !== _client.getUrl()) {
    _client.connect(url)
  }
  return _client
}
