export type EngineInfo = {
  version: string;
  platform: string;
  arch: string;
  serialPortCount: number;
  modbusTcpSlave: {
    address: string;
    stats: { clients: number; requests: number; errors: number };
  };
  capabilities: Record<string, boolean>;
  modbusRtu: { masterOpen: boolean; slaveOpen: boolean; stats: { requests: number; errors: number } };
};

export type SerialConfig = { port: string; baudRate: number; dataBits: number; parity: string; stopBits: string; timeoutMs?: number };


export type SimulatorRange = {
  label: string;
  table: "holding" | "input" | "coil" | "discrete";
  functionCode: number;
  address: number;
  values: number[];
  writable: boolean;
  note?: string;
};

export type SimulatorDevice = {
  key: string;
  name: string;
  slaveId: number;
  description: string;
  ranges: SimulatorRange[];
};

export type SimulatorProfile = {
  id: string;
  name: string;
  description: string;
  devices: SimulatorDevice[];
};

export type ModbusResult = {
  functionCode: number;
  address: number;
  quantity: number;
  values: number[];
  hex: string[];
  binary: string[];
  latencyMs: number;
  rawRequestHex: string;
  rawResponseHex: string;
};

export class CoreClient {
  constructor(
    private readonly baseUrl: string,
    private readonly token: string,
  ) {}

  private async request<T>(path: string, init?: RequestInit): Promise<T> {
    const response = await fetch(`${this.baseUrl.replace(/\/$/, "")}${path}`, {
      ...init,
      headers: {
        "content-type": "application/json",
        authorization: `Bearer ${this.token}`,
        ...(init?.headers ?? {}),
      },
    });
    const payload = (await response.json().catch(() => ({}))) as { error?: string } & T;
    if (!response.ok) throw new Error(payload.error || `Core returned HTTP ${response.status}`);
    return payload;
  }

  engine() {
    return this.request<EngineInfo>("/api/v1/engine");
  }

  serialPorts() {
    return this.request<{ ports: Array<{ name: string }> }>("/api/v1/serial/ports");
  }

  simulatorProfile() {
    return this.request<SimulatorProfile>("/api/v1/simulator/profile");
  }

  resetSimulator() {
    return this.request<{ ok: boolean; profile: string }>("/api/v1/simulator/reset", { method: "POST" });
  }

  writeSimulatorRegisters(slaveId: number, table: string, address: number, values: number[]) {
    return this.request<{ ok: boolean; values: number[] }>("/api/v1/simulator/registers", {
      method: "PATCH",
      body: JSON.stringify({ slaveId, table, address, values }),
    });
  }

  modbus(body: Record<string, unknown>) {
    return this.request<ModbusResult>("/api/v1/modbus/request", { method: "POST", body: JSON.stringify(body) });
  }

  openRtuMaster(config: SerialConfig) {
    return this.request<{ ok: boolean }>("/api/v1/rtu/master/open", { method: "POST", body: JSON.stringify(config) });
  }

  closeRtuMaster() { return this.request<{ ok: boolean }>("/api/v1/rtu/master", { method: "DELETE" }); }

  rtu(body: Record<string, unknown>) {
    return this.request<ModbusResult>("/api/v1/rtu/request", { method: "POST", body: JSON.stringify(body) });
  }

  startRtuSlave(config: SerialConfig) {
    return this.request<{ ok: boolean }>("/api/v1/rtu/slave/start", { method: "POST", body: JSON.stringify(config) });
  }

  stopRtuSlave() { return this.request<{ ok: boolean }>("/api/v1/rtu/slave", { method: "DELETE" }); }

  readSlave(slaveId: number, table: string, address: number, quantity: number) {
    const query = new URLSearchParams({ table, address: String(address), quantity: String(quantity) });
    return this.request<{ values: number[]; slaveId: number; table: string; address: number; quantity: number }>(
      `/api/v1/slaves/${slaveId}/registers?${query}`,
    );
  }

  writeSlave(slaveId: number, table: string, address: number, values: number[]) {
    return this.request<{ values: number[] }>(`/api/v1/slaves/${slaveId}/registers`, {
      method: "PATCH",
      body: JSON.stringify({ table, address, values }),
    });
  }
}
