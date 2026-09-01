import type { ModbusResult, SerialConfig } from "@/lib/core";

type WebSerialPort = {
  readable: ReadableStream<Uint8Array> | null;
  writable: WritableStream<Uint8Array> | null;
  open(options: { baudRate: number; dataBits?: number; stopBits?: number; parity?: "none" | "even" | "odd"; flowControl?: "none" | "hardware" }): Promise<void>;
  close(): Promise<void>;
  getInfo(): { usbVendorId?: number; usbProductId?: number };
};
type WebSerial = { requestPort(): Promise<WebSerialPort> };
declare global { interface Navigator { serial?: WebSerial } }

type RequestBody = { slaveId: number; functionCode: number; address: number; quantity?: number; value?: number; values?: number[]; timeoutMs?: number };

const u16 = (value: number): [number, number] => [(value >> 8) & 0xff, value & 0xff];
function crc16(data: Uint8Array) { let crc = 0xffff; for (const byte of data) { crc ^= byte; for (let bit = 0; bit < 8; bit += 1) crc = (crc & 1) ? (crc >> 1) ^ 0xa001 : crc >> 1; } return crc & 0xffff; }
function withCrc(bytes: number[]) { const body = Uint8Array.from(bytes); const crc = crc16(body); return Uint8Array.from([...body, crc & 0xff, (crc >> 8) & 0xff]); }
function hex(data: Uint8Array) { return Array.from(data, (byte) => byte.toString(16).toUpperCase().padStart(2, "0")).join(" "); }

function buildFrame(req: RequestBody) {
  const { slaveId, functionCode: fc, address } = req;
  if (!Number.isInteger(slaveId) || slaveId < 1 || slaveId > 247) throw new Error("Slave ID must be 1..247");
  if (!Number.isInteger(address) || address < 0 || address > 65535) throw new Error("Address must be 0..65535");
  const [ah, al] = u16(address);
  if ([1,2,3,4].includes(fc)) { const quantity = Number(req.quantity ?? 0); const max = fc <= 2 ? 2000 : 125; if (!Number.isInteger(quantity) || quantity < 1 || quantity > max) throw new Error(`Quantity must be 1..${max}`); const [qh, ql] = u16(quantity); return { frame: withCrc([slaveId,fc,ah,al,qh,ql]), quantity, values: [] as number[] }; }
  if (fc === 5) { const raw = Number(req.value ?? 0) === 0 ? 0 : 0xff00; const [vh,vl] = u16(raw); return { frame: withCrc([slaveId,fc,ah,al,vh,vl]), quantity: 1, values: [raw ? 1 : 0] }; }
  if (fc === 6) { const value = Number(req.value ?? 0); if (!Number.isInteger(value) || value < 0 || value > 65535) throw new Error("Value must be 0..65535"); const [vh,vl] = u16(value); return { frame: withCrc([slaveId,fc,ah,al,vh,vl]), quantity: 1, values: [value] }; }
  const values = req.values ?? [];
  if (fc === 15) { if (!values.length || values.length > 1968) throw new Error("FC15 requires 1..1968 values"); const count = Math.ceil(values.length / 8); const packed = new Array<number>(count).fill(0); values.forEach((v,i) => { if (v) packed[Math.floor(i/8)] |= 1 << (i%8); }); const [qh,ql] = u16(values.length); return { frame: withCrc([slaveId,fc,ah,al,qh,ql,count,...packed]), quantity: values.length, values: [...values] }; }
  if (fc === 16) { if (!values.length || values.length > 123) throw new Error("FC16 requires 1..123 values"); const payload = values.flatMap((v) => { if (!Number.isInteger(v) || v < 0 || v > 65535) throw new Error("Register values must be 0..65535"); return u16(v); }); const [qh,ql] = u16(values.length); return { frame: withCrc([slaveId,fc,ah,al,qh,ql,payload.length,...payload]), quantity: values.length, values: [...values] }; }
  throw new Error(`Unsupported function code ${fc}`);
}

function expectedLength(bytes: number[]) { if (bytes.length < 2) return null; if (bytes[1] & 0x80) return 5; if ([1,2,3,4].includes(bytes[1])) return bytes.length >= 3 ? 5 + bytes[2] : null; if ([5,6,15,16].includes(bytes[1])) return 8; return null; }
function parseResponse(req: RequestBody, response: Uint8Array, quantity: number, writeValues: number[]) {
  if (response.length < 5) throw new Error("Truncated RTU response");
  const got = response[response.length-2] | (response[response.length-1] << 8); if (crc16(response.slice(0,-2)) !== got) throw new Error("RTU CRC mismatch");
  if (response[0] !== req.slaveId) throw new Error("RTU slave ID mismatch"); const fc = response[1]; if (fc & 0x80) throw new Error(`Modbus exception ${response[2]}`); if (fc !== req.functionCode) throw new Error("RTU function code mismatch");
  if (fc === 1 || fc === 2) return Array.from({length: quantity}, (_,i) => (response[3 + Math.floor(i/8)] >> (i%8)) & 1);
  if (fc === 3 || fc === 4) { if (response[2] !== quantity*2) throw new Error("Unexpected RTU byte count"); return Array.from({length: quantity}, (_,i) => (response[3+i*2] << 8) | response[4+i*2]); }
  if (fc === 5) return [response[4] === 0xff && response[5] === 0 ? 1 : 0]; if (fc === 6) return [(response[4] << 8) | response[5]]; return writeValues;
}

export function webSerialSupported() { return typeof navigator !== "undefined" && Boolean(navigator.serial?.requestPort); }
export class BrowserRtuMaster {
  private port: WebSerialPort | null = null;
  private queue = Promise.resolve();
  async connect(config: SerialConfig) {
    if (!navigator.serial?.requestPort) throw new Error("Web Serial is not supported. Use Chrome or Edge desktop over HTTPS/localhost.");
    if (!["none","even","odd"].includes(config.parity)) throw new Error("Browser USB supports none/even/odd parity only. Use Native Core for mark/space.");
    if (!["1","2"].includes(config.stopBits)) throw new Error("Browser USB supports 1 or 2 stop bits only. Use Native Core for 1.5 stop bits.");
    const port = await navigator.serial.requestPort();
    await port.open({ baudRate: config.baudRate, dataBits: config.dataBits, parity: config.parity as "none"|"even"|"odd", stopBits: Number(config.stopBits), flowControl: "none" });
    this.port = port; const info = port.getInfo(); const v = info.usbVendorId?.toString(16).toUpperCase().padStart(4,"0"); const p = info.usbProductId?.toString(16).toUpperCase().padStart(4,"0"); return v || p ? `USB ${v ?? "----"}:${p ?? "----"}` : "Browser serial device";
  }
  async disconnect() { const port = this.port; this.port = null; if (port) await port.close(); }
  request(body: Record<string, unknown>): Promise<ModbusResult> { const op = this.queue.then(() => this.execute(body as unknown as RequestBody)); this.queue = op.then(() => undefined, () => undefined); return op; }
  private async execute(req: RequestBody): Promise<ModbusResult> {
    const port = this.port; if (!port?.readable || !port.writable) throw new Error("USB device is not connected");
    const { frame, quantity, values: writeValues } = buildFrame(req); const started = performance.now(); const writer = port.writable.getWriter(); try { await writer.write(frame); } finally { writer.releaseLock(); }
    const reader = port.readable.getReader(); const bytes: number[] = []; const timeoutMs = Math.min(60000, Math.max(100, Number(req.timeoutMs ?? 1500))); const deadline = performance.now() + timeoutMs;
    try { while (performance.now() < deadline) { const remaining = Math.max(1, deadline - performance.now()); const chunk = await Promise.race([reader.read(), new Promise<never>((_,reject) => window.setTimeout(() => reject(new Error(`Serial read timeout after ${timeoutMs} ms`)), remaining))]); if (chunk.done) throw new Error("USB serial stream closed"); if (chunk.value) bytes.push(...chunk.value); const length = expectedLength(bytes); if (length !== null && bytes.length >= length) break; } } finally { reader.releaseLock(); }
    const length = expectedLength(bytes); if (length === null || bytes.length < length) throw new Error(`Serial read timeout after ${timeoutMs} ms`); const response = Uint8Array.from(bytes.slice(0,length)); const values = parseResponse(req,response,quantity,writeValues);
    return { functionCode: req.functionCode, address: req.address, quantity: values.length, values, hex: values.map((v) => `0x${v.toString(16).toUpperCase().padStart(4,"0")}`), binary: values.map((v) => v.toString(2).padStart(16,"0")), latencyMs: Math.round(performance.now()-started), rawRequestHex: hex(frame), rawResponseHex: hex(response) };
  }
}
