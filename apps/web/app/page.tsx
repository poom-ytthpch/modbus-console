"use client";

import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { CoreClient, EngineInfo, ModbusResult, SerialConfig, SimulatorProfile } from "@/lib/core";

type Connection = { url: string; token: string };

const readFunctions = new Set([1, 2, 3, 4]);
const singleWriteFunctions = new Set([5, 6]);

function parseValues(text: string): number[] {
  if (!text.trim()) return [];
  return text.split(/[\s,]+/).filter(Boolean).map((part) => {
    const value = Number(part);
    if (!Number.isInteger(value) || value < 0 || value > 65535) throw new Error(`Invalid value: ${part}`);
    return value;
  });
}

export default function Home() {
  const [connection, setConnection] = useState<Connection>({ url: "http://127.0.0.1:17777", token: "" });
  const [engine, setEngine] = useState<EngineInfo | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<ModbusResult | null>(null);
  const [autoPoll, setAutoPoll] = useState(false);
  const [pollInterval, setPollInterval] = useState(1000);
  const [serialPorts, setSerialPorts] = useState<string[]>([]);
  const [simulator, setSimulator] = useState<SimulatorProfile | null>(null);
  const [simSelection, setSimSelection] = useState("");
  const [simValues, setSimValues] = useState("");
  const [transport, setTransport] = useState<"tcp" | "rtu">("tcp");
  const [activeView, setActiveView] = useState<"poll" | "slave" | "simulator" | "rtu">("poll");
  const [serial, setSerial] = useState<SerialConfig>({ port: "", baudRate: 9600, dataBits: 8, parity: "none", stopBits: "1", timeoutMs: 1500 });

  const [poll, setPoll] = useState({ host: "127.0.0.1", port: 1502, slaveId: 5, functionCode: 3, address: 1, quantity: 8, value: 256, values: "256,512" });
  const [slaveView, setSlaveView] = useState({ slaveId: 5, table: "holding", address: 1, quantity: 8, values: "" });
  const [slaveValues, setSlaveValues] = useState<number[]>([]);

  const client = useMemo(() => new CoreClient(connection.url, connection.token), [connection]);
  const simulatorRanges = useMemo(() => simulator?.devices.flatMap((device) => device.ranges.map((range, index) => ({ device, range, key: `${device.key}:${index}` }))) ?? [], [simulator]);
  const selectedSimulatorRange = simulatorRanges.find((item) => item.key === simSelection) ?? simulatorRanges[0];

  async function run<T>(operation: () => Promise<T>, success: (value: T) => void) {
    setBusy(true);
    setError("");
    try {
      success(await operation());
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setBusy(false);
    }
  }

  function connect(event: FormEvent) {
    event.preventDefault();
    sessionStorage.setItem("modbus-console.engine-url", connection.url);
    sessionStorage.setItem("modbus-console.pairing-token", connection.token);
    void run(async () => {
      const [engineInfo, profile] = await Promise.all([client.engine(), client.simulatorProfile()]);
      return { engineInfo, profile };
    }, ({ engineInfo, profile }) => {
      setEngine(engineInfo);
      setSimulator(profile);
      const first = profile.devices[0]?.ranges[0];
      if (first) {
        setSimSelection(`${profile.devices[0].key}:0`);
        setSimValues(first.values.join(","));
      }
    });
  }

  function chooseSimulatorRange(key: string) {
    setSimSelection(key);
    const item = simulatorRanges.find((candidate) => candidate.key === key);
    if (item) setSimValues(item.range.values.join(","));
  }

  function loadSimulatorInPoll() {
    if (!selectedSimulatorRange) return;
    const { device, range } = selectedSimulatorRange;
    setTransport("tcp");
    setPoll((current) => ({ ...current, host: "127.0.0.1", port: 1502, slaveId: device.slaveId, functionCode: range.functionCode, address: range.address, quantity: range.values.length }));
    setSlaveView((current) => ({ ...current, slaveId: device.slaveId, table: range.table, address: range.address, quantity: range.values.length }));
    setActiveView("poll");
  }

  function injectSimulatorState() {
    if (!selectedSimulatorRange) return;
    const { device, range } = selectedSimulatorRange;
    const values = parseValues(simValues);
    void run(() => client.writeSimulatorRegisters(device.slaveId, range.table, range.address, values), (value) => setSimValues(value.values.join(",")));
  }

  function resetSimulator() {
    void run(() => client.resetSimulator(), () => {
      if (selectedSimulatorRange) setSimValues(selectedSimulatorRange.range.values.join(","));
      setResult(null);
      setSlaveValues([]);
    });
  }

  const requestBody = useCallback(() => {
    const fc = poll.functionCode;
    const body: Record<string, unknown> = { host: poll.host, port: poll.port, slaveId: poll.slaveId, functionCode: fc, address: poll.address, timeoutMs: serial.timeoutMs ?? 1500 };
    if (readFunctions.has(fc)) body.quantity = poll.quantity;
    else if (singleWriteFunctions.has(fc)) body.value = poll.value;
    else body.values = parseValues(poll.values);
    return body;
  }, [poll.host, poll.port, poll.slaveId, poll.functionCode, poll.address, poll.quantity, poll.value, poll.values, serial.timeoutMs]);

  const executePoll = useCallback(() => {
    const body = requestBody();
    return transport === "tcp" ? client.modbus(body) : client.rtu(body);
  }, [client, requestBody, transport]);

  function sendModbus(event: FormEvent) {
    event.preventDefault();
    void run(executePoll, setResult);
  }

  function refreshSerialPorts() {
    void run(() => client.serialPorts(), (value) => {
      const ports = value.ports.map((port) => port.name);
      setSerialPorts(ports);
      if (!serial.port && ports[0]) setSerial((current) => ({ ...current, port: ports[0] }));
    });
  }

  function connectUsb() {
    void run(() => client.openRtuMaster(serial), () => client.engine().then(setEngine));
  }

  function disconnectUsb() {
    setAutoPoll(false);
    void run(() => client.closeRtuMaster(), () => client.engine().then(setEngine));
  }

  useEffect(() => {
    if (!autoPoll || !engine || !readFunctions.has(poll.functionCode)) return;
    let cancelled = false;
    const tick = async () => {
      try {
        const value = await executePoll();
        if (!cancelled) { setResult(value); setError(""); }
      } catch (cause) {
        if (!cancelled) { setError(cause instanceof Error ? cause.message : String(cause)); setAutoPoll(false); }
      }
    };
    void tick();
    const timer = window.setInterval(() => void tick(), Math.max(100, pollInterval));
    return () => { cancelled = true; window.clearInterval(timer); };
  }, [autoPoll, pollInterval, poll.functionCode, engine, executePoll]);

  function readSlave(event: FormEvent) {
    event.preventDefault();
    void run(() => client.readSlave(slaveView.slaveId, slaveView.table, slaveView.address, slaveView.quantity), (value) => setSlaveValues(value.values));
  }

  function writeSlave() {
    const values = parseValues(slaveView.values);
    void run(() => client.writeSlave(slaveView.slaveId, slaveView.table, slaveView.address, values), (value) => setSlaveValues(value.values));
  }

  return (
    <main className="consoleShell">
      <header className="appBar">
        <div className="brandBlock">
          <strong>Modbus Console</strong>
          <span>Poll · Slave · RTU</span>
        </div>
        <div className={`coreState ${engine ? "online" : ""}`}>
          <span className="coreDot" />
          <div>
            <strong>{engine ? "Core connected" : "Core offline"}</strong>
            <small>{engine ? `${engine.platform}/${engine.arch} · ${engine.version}` : "Pair the local Core Engine to begin"}</small>
          </div>
        </div>
      </header>

      <section className="engineBar" aria-label="Core Engine connection">
        <form className="engineForm" onSubmit={connect}>
          <label className="field grow">
            <span>Core URL</span>
            <input value={connection.url} onChange={(e) => setConnection({ ...connection, url: e.target.value })} placeholder="https://engine.example.com" />
          </label>
          <label className="field tokenField">
            <span>Pairing token</span>
            <input type="password" value={connection.token} onChange={(e) => setConnection({ ...connection, token: e.target.value })} autoComplete="off" />
          </label>
          <button className="primaryAction" disabled={busy || !connection.token}>Connect</button>
        </form>
        {engine ? (
          <div className="engineSummary">
            <MiniStatus label="TCP Slave" value={engine.modbusTcpSlave.address} />
            <MiniStatus label="RTU Master" value={engine.modbusRtu.masterOpen ? "OPEN" : "CLOSED"} active={engine.modbusRtu.masterOpen} />
            <MiniStatus label="RTU Slave" value={engine.modbusRtu.slaveOpen ? "RUNNING" : "STOPPED"} active={engine.modbusRtu.slaveOpen} />
            <MiniStatus label="Serial" value={`${engine.serialPortCount} port${engine.serialPortCount === 1 ? "" : "s"}`} />
          </div>
        ) : (
          <p className="engineHint">Local: http://127.0.0.1:17777 · Vercel: use the HTTPS Core tunnel URL.</p>
        )}
      </section>

      {error && <div className="alert" role="alert">{error}</div>}

      <nav className="workspaceTabs" aria-label="Modbus workspace">
        <WorkspaceTab active={activeView === "poll"} label="Modbus Poll" detail="Master / Client" onClick={() => setActiveView("poll")} />
        <WorkspaceTab active={activeView === "slave"} label="Modbus Slave" detail="Server / Map" onClick={() => setActiveView("slave")} />
        <WorkspaceTab active={activeView === "simulator"} label="Virtual Lab" detail={simulator ? `${simulator.devices.length} devices` : "Simulator"} onClick={() => setActiveView("simulator")} />
        <WorkspaceTab active={activeView === "rtu"} label="USB / RS485" detail="Serial RTU" onClick={() => setActiveView("rtu")} />
      </nav>

      <section className="workspace">
        {activeView === "poll" && (
          <div className="toolLayout">
            <aside className="settingsPane">
              <ToolHeading eyebrow="MASTER / CLIENT" title="Modbus Poll" meta="FC01–06 · 15 · 16" />

              <div className="segmented" aria-label="Transport">
                <button type="button" className={transport === "tcp" ? "selected" : ""} onClick={() => setTransport("tcp")}>TCP</button>
                <button type="button" className={transport === "rtu" ? "selected" : ""} onClick={() => setTransport("rtu")}>RTU</button>
              </div>

              <form className="settingsForm" onSubmit={sendModbus}>
                {transport === "tcp" && (
                  <div className="fieldGrid two">
                    <label className="field span2"><span>Host</span><input value={poll.host} onChange={(e) => setPoll({ ...poll, host: e.target.value })} /></label>
                    <label className="field"><span>Port</span><input type="number" value={poll.port} onChange={(e) => setPoll({ ...poll, port: Number(e.target.value) })} /></label>
                    <label className="field"><span>Slave ID</span><input type="number" min="1" max="247" value={poll.slaveId} onChange={(e) => setPoll({ ...poll, slaveId: Number(e.target.value) })} /></label>
                  </div>
                )}
                {transport === "rtu" && <label className="field"><span>Slave ID</span><input type="number" min="1" max="247" value={poll.slaveId} onChange={(e) => setPoll({ ...poll, slaveId: Number(e.target.value) })} /></label>}

                {transport === "rtu" && (
                  <div className="writeBox">
                    <label className="field">
                      <span>USB / Serial port</span>
                      <select value={serial.port} disabled={engine?.modbusRtu.masterOpen} onChange={(e) => setSerial({ ...serial, port: e.target.value })}>
                        <option value="">Select USB-RS485 device</option>
                        {serialPorts.map((port) => <option key={port}>{port}</option>)}
                      </select>
                    </label>
                    <div className="modeActions">
                      <button type="button" className="secondaryAction" disabled={busy || !engine || engine?.modbusRtu.masterOpen} onClick={refreshSerialPorts}>Refresh USB</button>
                      {engine?.modbusRtu.masterOpen ? (
                        <button type="button" className="dangerAction" disabled={busy} onClick={disconnectUsb}>Disconnect USB</button>
                      ) : (
                        <button type="button" className="primaryAction" disabled={busy || !engine || !serial.port || engine?.modbusRtu.slaveOpen} onClick={connectUsb}>Connect USB</button>
                      )}
                    </div>
                    <p>{engine?.modbusRtu.masterOpen ? `Connected to ${serial.port}` : "Connect a USB-RS485 adapter before sending RTU requests."}</p>
                  </div>
                )}

                <div className="fieldGrid two">
                  <label className="field"><span>Function</span><select value={poll.functionCode} onChange={(e) => setPoll({ ...poll, functionCode: Number(e.target.value) })}>{[1,2,3,4,5,6,15,16].map((fc) => <option key={fc} value={fc}>FC{String(fc).padStart(2,"0")}</option>)}</select></label>
                  <label className="field"><span>Address</span><input type="number" min="0" max="65535" value={poll.address} onChange={(e) => setPoll({ ...poll, address: Number(e.target.value) })} /></label>
                  {readFunctions.has(poll.functionCode) && <label className="field"><span>Quantity</span><input type="number" min="1" value={poll.quantity} onChange={(e) => setPoll({ ...poll, quantity: Number(e.target.value) })} /></label>}
                  {singleWriteFunctions.has(poll.functionCode) && <label className="field"><span>Value</span><input type="number" value={poll.value} onChange={(e) => setPoll({ ...poll, value: Number(e.target.value) })} /></label>}
                  {[15,16].includes(poll.functionCode) && <label className="field span2"><span>Values</span><input value={poll.values} onChange={(e) => setPoll({ ...poll, values: e.target.value })} placeholder="100,200,300" /></label>}
                </div>

                <button className="primaryAction full" disabled={busy || !engine || (transport === "rtu" && !engine.modbusRtu.masterOpen)}>{[5,6,15,16].includes(poll.functionCode) ? "Write request" : "Read once"}</button>
              </form>

              <div className="pollBox">
                <label className="field"><span>Poll interval</span><div className="inputSuffix"><input type="number" min="100" max="60000" value={pollInterval} onChange={(e) => setPollInterval(Number(e.target.value))} /><span>ms</span></div></label>
                <button type="button" className={autoPoll ? "dangerAction" : "secondaryAction"} disabled={!engine || !readFunctions.has(poll.functionCode) || (transport === "rtu" && !engine.modbusRtu.masterOpen)} onClick={() => setAutoPoll((value) => !value)}>{autoPoll ? "Stop polling" : "Start polling"}</button>
              </div>
            </aside>

            <div className="dataPane">
              <div className="dataToolbar">
                <div>
                  <strong>Register values</strong>
                  <span>{result ? `${transport.toUpperCase()} · Slave ${poll.slaveId} · FC${String(poll.functionCode).padStart(2,"0")}` : "Run a request to inspect register data"}</span>
                </div>
                {result && <div className="dataStats"><span>{result.quantity} values</span><span>{result.latencyMs} ms</span></div>}
              </div>
              {result ? <RegisterTable values={result.values} start={result.address} /> : <EmptyWorkspace title="No register data" text="Configure the request on the left and read once or start polling." />}
              {result && <div className="frameLog"><Raw label="TX" value={result.rawRequestHex} /><Raw label="RX" value={result.rawResponseHex} /></div>}
            </div>
          </div>
        )}

        {activeView === "slave" && (
          <div className="toolLayout">
            <aside className="settingsPane">
              <ToolHeading eyebrow="SERVER / SIMULATOR" title="Modbus Slave" meta="Core TCP :1502" />
              <form className="settingsForm" onSubmit={readSlave}>
                <div className="fieldGrid two">
                  <label className="field"><span>Slave ID</span><input type="number" min="1" max="247" value={slaveView.slaveId} onChange={(e) => setSlaveView({ ...slaveView, slaveId: Number(e.target.value) })} /></label>
                  <label className="field"><span>Table</span><select value={slaveView.table} onChange={(e) => setSlaveView({ ...slaveView, table: e.target.value })}>{["holding","input","coil","discrete"].map((table) => <option key={table}>{table}</option>)}</select></label>
                  <label className="field"><span>Address</span><input type="number" min="0" value={slaveView.address} onChange={(e) => setSlaveView({ ...slaveView, address: Number(e.target.value) })} /></label>
                  <label className="field"><span>Quantity</span><input type="number" min="1" value={slaveView.quantity} onChange={(e) => setSlaveView({ ...slaveView, quantity: Number(e.target.value) })} /></label>
                </div>
                <button className="primaryAction full" disabled={busy || !engine}>Read register map</button>
              </form>

              <div className="writeBox">
                <label className="field"><span>Write raw values</span><input value={slaveView.values} onChange={(e) => setSlaveView({ ...slaveView, values: e.target.value })} placeholder="256,512" /></label>
                <button type="button" className="dangerAction" disabled={busy || !engine || !["holding","coil"].includes(slaveView.table)} onClick={writeSlave}>Write values</button>
                <p>Writes are enabled only for Holding Registers and Coils.</p>
              </div>
            </aside>

            <div className="dataPane">
              <div className="dataToolbar">
                <div>
                  <strong>{slaveView.table.toUpperCase()} register map</strong>
                  <span>Slave {slaveView.slaveId} · Address {slaveView.address} · Quantity {slaveView.quantity}</span>
                </div>
              </div>
              {slaveValues.length ? <RegisterTable values={slaveValues} start={slaveView.address} /> : <EmptyWorkspace title="Register map not loaded" text="Read the selected map to inspect the current slave state." />}
              <div className="contextNote">SWS relay profile: slave 5 · COIL 0–7 active-low · HREG 1–8 synchronized with 256 / 512.</div>
            </div>
          </div>
        )}

        {activeView === "simulator" && (
          <div className="simulatorWorkspace">
            <aside className="deviceRail">
              <ToolHeading eyebrow="VIRTUAL LAB" title={simulator?.name ?? "Simulator"} meta={simulator ? `${simulator.devices.length} devices` : "Core required"} />
              {simulator ? (
                <div className="deviceList">
                  {simulator.devices.map((device) => (
                    <button type="button" key={device.key} className={selectedSimulatorRange?.device.key === device.key ? "deviceRow selected" : "deviceRow"} onClick={() => chooseSimulatorRange(`${device.key}:0`)}>
                      <span><strong>{device.name}</strong><small>{device.description}</small></span>
                      <b>ID {device.slaveId}</b>
                    </button>
                  ))}
                </div>
              ) : <EmptyRail text="Connect Core to load the simulator profile." />}
              {simulator && <button type="button" className="dangerAction full" disabled={busy || !engine} onClick={resetSimulator}>Reset lab state</button>}
            </aside>

            <div className="simulatorDetail">
              {selectedSimulatorRange ? (
                <>
                  <div className="dataToolbar">
                    <div>
                      <strong>{selectedSimulatorRange.device.name}</strong>
                      <span>Slave {selectedSimulatorRange.device.slaveId} · TCP :1502</span>
                    </div>
                    <button type="button" className="secondaryAction" onClick={loadSimulatorInPoll}>Load in Poll</button>
                  </div>
                  <div className="simEditor">
                    <label className="field"><span>Register range</span><select value={selectedSimulatorRange.key} onChange={(e) => chooseSimulatorRange(e.target.value)}>{simulatorRanges.map(({ device, range, key }) => <option key={key} value={key}>{device.name} · ID {device.slaveId} · {range.label}</option>)}</select></label>
                    <div className="rangeMeta">
                      <MetaCell label="Function" value={`FC${String(selectedSimulatorRange.range.functionCode).padStart(2,"0")}`} />
                      <MetaCell label="Table" value={selectedSimulatorRange.range.table.toUpperCase()} />
                      <MetaCell label="Address" value={String(selectedSimulatorRange.range.address)} />
                      <MetaCell label="Quantity" value={String(selectedSimulatorRange.range.values.length)} />
                    </div>
                    <label className="field"><span>Simulated raw values</span><input value={simValues} onChange={(e) => setSimValues(e.target.value)} placeholder="285,720" /></label>
                    <button type="button" className="primaryAction fit" disabled={busy || !engine} onClick={injectSimulatorState}>Inject state</button>
                    {selectedSimulatorRange.range.note && <p className="contextNote">{selectedSimulatorRange.range.note}</p>}
                  </div>

                  <div className="rangeList">
                    {selectedSimulatorRange.device.ranges.map((range, index) => (
                      <button type="button" key={`${range.table}-${range.address}`} className={`${selectedSimulatorRange.key === `${selectedSimulatorRange.device.key}:${index}` ? "active " : ""}rangeRow`} onClick={() => chooseSimulatorRange(`${selectedSimulatorRange.device.key}:${index}`)}>
                        <span>{range.label}</span><code>FC{String(range.functionCode).padStart(2,"0")} · {range.table.toUpperCase()} · @{range.address}</code><strong>{range.values.join(", ")}</strong>
                      </button>
                    ))}
                  </div>
                </>
              ) : <EmptyWorkspace title="No simulator selected" text="Connect Core to load virtual Modbus devices." />}
            </div>
          </div>
        )}

        {activeView === "rtu" && (
          <div className="rtuWorkspace">
            <div className="rtuHeader">
              <ToolHeading eyebrow="USB / RS485" title="Modbus RTU" meta="Native Core serial access" />
              <button type="button" className="secondaryAction" disabled={busy || !engine || engine?.modbusRtu.masterOpen || engine?.modbusRtu.slaveOpen} onClick={refreshSerialPorts}>Refresh USB devices</button>
            </div>
            <div className="serialGrid">
              <label className="field wide"><span>Serial port</span><select value={serial.port} onChange={(e) => setSerial({ ...serial, port: e.target.value })}><option value="">Select port</option>{serialPorts.map((port) => <option key={port}>{port}</option>)}</select></label>
              <label className="field"><span>Baud</span><select value={serial.baudRate} onChange={(e) => setSerial({ ...serial, baudRate: Number(e.target.value) })}>{[1200,2400,4800,9600,19200,38400,57600,115200].map((v)=><option key={v}>{v}</option>)}</select></label>
              <label className="field"><span>Data bits</span><select value={serial.dataBits} onChange={(e) => setSerial({ ...serial, dataBits: Number(e.target.value) })}>{[7,8].map((v)=><option key={v}>{v}</option>)}</select></label>
              <label className="field"><span>Parity</span><select value={serial.parity} onChange={(e) => setSerial({ ...serial, parity: e.target.value })}>{["none","even","odd","mark","space"].map((v)=><option key={v}>{v}</option>)}</select></label>
              <label className="field"><span>Stop bits</span><select value={serial.stopBits} onChange={(e) => setSerial({ ...serial, stopBits: e.target.value })}>{["1","1.5","2"].map((v)=><option key={v}>{v}</option>)}</select></label>
              <label className="field"><span>Timeout</span><div className="inputSuffix"><input type="number" min="100" max="60000" value={serial.timeoutMs} onChange={(e) => setSerial({ ...serial, timeoutMs: Number(e.target.value) })} /><span>ms</span></div></label>
            </div>

            <div className="rtuModeGrid">
              <div className="modePanel">
                <div><strong>RTU Master</strong><span>{engine?.modbusRtu.masterOpen ? "Serial session open" : "Use Poll with RTU transport"}</span></div>
                <span className={`modeBadge ${engine?.modbusRtu.masterOpen ? "live" : ""}`}>{engine?.modbusRtu.masterOpen ? "OPEN" : "CLOSED"}</span>
                <div className="modeActions">
                  <button type="button" className="primaryAction" disabled={busy || !engine || !serial.port || engine?.modbusRtu.slaveOpen || engine?.modbusRtu.masterOpen} onClick={connectUsb}>Connect USB</button>
                  <button type="button" className="dangerAction" disabled={busy || !engine?.modbusRtu.masterOpen} onClick={disconnectUsb}>Disconnect USB</button>
                </div>
              </div>
              <div className="modePanel">
                <div><strong>RTU Slave</strong><span>Expose the Core register map over USB-RS485</span></div>
                <span className={`modeBadge ${engine?.modbusRtu.slaveOpen ? "live" : ""}`}>{engine?.modbusRtu.slaveOpen ? "RUNNING" : "STOPPED"}</span>
                <div className="modeActions">
                  <button type="button" className="primaryAction" disabled={busy || !engine || !serial.port || engine?.modbusRtu.masterOpen || engine?.modbusRtu.slaveOpen} onClick={() => void run(() => client.startRtuSlave(serial), () => client.engine().then(setEngine))}>Start Slave</button>
                  <button type="button" className="dangerAction" disabled={busy || !engine?.modbusRtu.slaveOpen} onClick={() => void run(() => client.stopRtuSlave(), () => client.engine().then(setEngine))}>Stop Slave</button>
                </div>
              </div>
            </div>

            <div className="portStrip">{serialPorts.length ? serialPorts.map((port) => <code key={port}>{port}</code>) : <span>No serial ports loaded. Connect a USB-RS485 adapter and refresh.</span>}</div>
            <p className="contextNote">A serial port is exclusive. Close RTU Master before starting RTU Slave, and stop RTU Slave before opening Master.</p>
          </div>
        )}
      </section>

      <footer className="statusBar">
        <span><i className={engine ? "live" : ""} />{engine ? "Core online" : "Core offline"}</span>
        <span>{transport.toUpperCase()}</span>
        <span>Slave {poll.slaveId}</span>
        <span>{autoPoll ? `Polling ${pollInterval} ms` : "Polling stopped"}</span>
        <span className="statusSpacer" />
        <span>Modbus Console 0.1</span>
      </footer>
    </main>
  );
}

function WorkspaceTab({ active, label, detail, onClick }: { active: boolean; label: string; detail: string; onClick: () => void }) {
  return <button type="button" className={active ? "workspaceTab active" : "workspaceTab"} onClick={onClick}><strong>{label}</strong><span>{detail}</span></button>;
}

function ToolHeading({ eyebrow, title, meta }: { eyebrow: string; title: string; meta: string }) {
  return <div className="toolHeading"><span>{eyebrow}</span><div><strong>{title}</strong><small>{meta}</small></div></div>;
}

function MiniStatus({ label, value, active = false }: { label: string; value: string; active?: boolean }) {
  return <div className={`miniStatus ${active ? "active" : ""}`}><span>{label}</span><strong>{value}</strong></div>;
}

function MetaCell({ label, value }: { label: string; value: string }) {
  return <div className="metaCell"><span>{label}</span><strong>{value}</strong></div>;
}

function EmptyWorkspace({ title, text }: { title: string; text: string }) {
  return <div className="emptyWorkspace"><div className="emptyGrid" /><strong>{title}</strong><span>{text}</span></div>;
}

function EmptyRail({ text }: { text: string }) {
  return <div className="emptyRail">{text}</div>;
}

function Raw({ label, value }: { label: string; value: string }) {
  return <div className="raw"><span>{label}</span><code>{value}</code></div>;
}

function RegisterTable({ values, start }: { values: number[]; start: number }) {
  return (
    <div className="registers">
      <div className="register row head"><span>Address</span><span>DEC</span><span>HEX</span><span>BIN</span></div>
      {values.map((value, index) => <div className="register row" key={`${start}-${index}`}><span>{start + index}</span><span>{value}</span><span>0x{value.toString(16).toUpperCase().padStart(4,"0")}</span><span>{value.toString(2).padStart(16,"0")}</span></div>)}
    </div>
  );
}
