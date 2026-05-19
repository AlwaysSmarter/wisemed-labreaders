const byId = (id) => document.getElementById(id);

function makeRequestId() {
  return `req-${Date.now()}-${Math.floor(Math.random() * 100000)}`;
}

function readerCommandSamples() {
  return {
    'reader.status': {},
    'stats.get': { order_date: new Date().toISOString().slice(0, 10) },
    'stats.series': { series_limit: 14 },
    'config.get': { section: 'reader' },
    'config.set': {
      section: 'reader',
      data: {
        label: 'Reader Test A',
      },
    },
    'logs.list': { limit: 25 },
    'logs.tail': { lines: 50 },
    'logs.activate': {},
    'logs.deactivate': {},
    'results.activate': {},
    'results.deactivate': {},
    'analytes.list': {},
    'analytes.get': { id: 1 },
    'analytes.create': {
      active: true,
      tag: 'SALM_D1',
      code: 'Salmonella O-groups v3|O:9 (D1)',
      name: 'Salmonella O-groups v3 | O:9 (D1)',
      description: 'Test analyte created from Test A',
      result_type: 'text',
      result_formatting: 'raw',
      result_weighting: 1,
      result_measure_unit: '',
      result_reagents_set: '',
    },
    'analytes.update': {
      id: 1,
      active: true,
      tag: 'SALM_D1',
      code: 'Salmonella O-groups v3|O:9 (D1)',
      name: 'Salmonella O-groups v3 | O:9 (D1)',
      description: 'Updated from Test A',
      result_type: 'text',
      result_formatting: 'raw',
      result_weighting: 1,
      result_measure_unit: '',
      result_reagents_set: '',
    },
    'analytes.delete': { id: 1 },
    'orders.list': {
      round_no: 1,
      order_date: new Date().toISOString().slice(0, 10),
      include_analysis: true,
    },
    'orders.rounds': {
      order_date: new Date().toISOString().slice(0, 10),
    },
    'orders.get': { id: 1 },
    'orders.create': {
      round_no: 1,
      order_date: new Date().toISOString().slice(0, 10),
      sample_id: '238886',
      file_id: '238886',
      patient_id: '',
      patient_name: 'Test Sample',
      rack_no: 1,
      rack_position: 0,
      list_position: 1,
      sample_no: 1,
      status: 'scheduled',
      source_file: 'manual-test.csv',
    },
    'orders.update': {
      round_no: 1,
      order_date: new Date().toISOString().slice(0, 10),
      sample_id: '238886',
      file_id: '238886',
      patient_id: '',
      patient_name: 'Updated Test Sample',
      rack_no: 1,
      rack_position: 0,
      list_position: 1,
      sample_no: 1,
      status: 'received',
      source_file: 'manual-test.csv',
    },
    'orders.delete': { id: 1 },
    'order_analysis.list': { order_id: 1 },
    'order_analysis.get': { id: 1 },
    'order_analysis.create': {
      order_id: 1,
      analyte_tag: 'SALM_D1',
      analyte_name: 'Salmonella O-groups v3 | O:9 (D1)',
      status: 'scheduled',
      result_value: '',
      raw_value: '',
      interpreted_value: '',
      unit: '',
      source_file: 'manual-test.csv',
    },
    'order_analysis.update': {
      id: 1,
      order_id: 1,
      analyte_tag: 'SALM_D1',
      analyte_name: 'Salmonella O-groups v3 | O:9 (D1)',
      status: 'completed',
      result_value: 'negative',
      raw_value: 'negative',
      interpreted_value: 'valid',
      unit: '',
      source_file: 'manual-test.csv',
    },
    'order_analysis.delete': { id: 1 },
    'results.list': { limit: 50 },
    'comm.get': {},
    'comm.set': {
      type: 'file',
      protocol: 'IRBIOTYPER',
    },
    'imports.run_file': {
      path: './inbox/sample-import.csv',
      order_date: new Date().toISOString().slice(0, 10),
    },
  };
}

function pageClientDefaults() {
  const params = new URLSearchParams(window.location.search);
  const path = window.location.pathname.toLowerCase();
  let defaultClientType = 'browser';
  let defaultClientId = `tab-${Math.floor(Math.random() * 1000)}`;
  let defaultLabel = document.title;
  let defaultReaderId = '';

  if (path.endsWith('/test-a.html')) {
    defaultClientId = 'browser-test-a';
    defaultLabel = 'Test A';
  } else if (path.endsWith('/test-b.html')) {
    defaultClientId = 'browser-test-b';
    defaultLabel = 'Test B';
  } else if (path.endsWith('/test-reader.html')) {
    defaultClientType = 'reader';
    defaultClientId = 'reader-file-001';
    defaultLabel = 'Reader Agent Demo';
    defaultReaderId = 'reader-file-001';
  }

  return {
    wsUrl: normalizeWSURL(params.get('ws') || `${window.location.host}/ws`),
    clientType: params.get('client_type') || defaultClientType,
    clientId: params.get('client_id') || defaultClientId,
    label: params.get('label') || defaultLabel,
    readerId: params.get('reader_id') || defaultReaderId,
  };
}

function normalizeWSURL(value) {
  const raw = (value || '').trim();
  if (!raw) {
    return `ws://${window.location.host}/ws`;
  }

  let normalized = raw;
  if (!/^wss?:\/\//i.test(normalized)) {
    const scheme = window.location.protocol === 'https:' ? 'wss://' : 'ws://';
    normalized = `${scheme}${normalized.replace(/^\/+/, '')}`;
  }

  try {
    const url = new URL(normalized);
    if (!url.pathname || url.pathname === '/') {
      url.pathname = '/ws';
    }
    return url.toString();
  } catch (_) {
    return normalized;
  }
}

function mountTestPage() {
  const defaults = pageClientDefaults();
  let ws = null;
  let myConnectionId = '';
  let authToken = '';
  const readerSamples = readerCommandSamples();

  byId('wsUrl').value = defaults.wsUrl;
  byId('clientType').value = defaults.clientType;
  byId('clientId').value = defaults.clientId;
  byId('label').value = defaults.label;
  if (byId('readerId')) {
    byId('readerId').value = defaults.readerId;
  }

  const log = (line) => {
    const out = byId('log');
    out.value += `[${new Date().toISOString()}] ${line}\n`;
    out.scrollTop = out.scrollHeight;
  };

  const setStatus = (line) => {
    const el = byId('status');
    if (el) {
      el.value = line;
    }
    log(line);
  };

  const send = (message) => {
    if (!ws || ws.readyState !== WebSocket.OPEN) {
      log('WS is not connected');
      return;
    }
    ws.send(JSON.stringify(message));
    log(`TX ${JSON.stringify(message)}`);
  };

  const setReaderResponse = (payload) => {
    const out = byId('readerApiResponse');
    if (!out) {
      return;
    }
    out.value = typeof payload === 'string' ? payload : JSON.stringify(payload, null, 2);
  };

  const currentReaderTarget = () => {
    const targetReaderEl = byId('apiTargetReaderId');
    if (targetReaderEl && targetReaderEl.value.trim()) {
      return targetReaderEl.value.trim();
    }
    if (byId('readerId') && byId('readerId').value.trim()) {
      return byId('readerId').value.trim();
    }
    return '';
  };

  const loadReaderSample = (commandName) => {
    const select = byId('readerCommand');
    const argsArea = byId('readerCommandArgs');
    if (!select || !argsArea) {
      return;
    }
    if (commandName) {
      select.value = commandName;
    }
    const sample = readerSamples[select.value] || {};
    argsArea.value = JSON.stringify(sample, null, 2);
  };

  const sendReaderCommand = () => {
    const select = byId('readerCommand');
    const argsArea = byId('readerCommandArgs');
    if (!select || !argsArea) {
      return;
    }
    let args = {};
    try {
      args = argsArea.value.trim() ? JSON.parse(argsArea.value) : {};
    } catch (error) {
      setStatus(`Invalid command args JSON: ${error.message}`);
      return;
    }
    const readerID = currentReaderTarget();
    if (!readerID) {
      setStatus('Target reader_id is required');
      return;
    }
    setReaderResponse({ pending: true, target_reader_id: readerID, command: select.value, args });
    send({
      type: 'command',
      request_id: makeRequestId(),
      target: {
        mode: 'reader',
        reader_id: readerID,
      },
      payload: {
        command: select.value,
        args,
      },
    });
  };

  const subjectForConnect = () => {
    const clientType = byId('clientType').value.trim();
    if (clientType === 'reader' && byId('readerId')) {
      return byId('readerId').value.trim();
    }
    return byId('clientId').value.trim();
  };

  const roleForConnect = () => {
    const clientType = byId('clientType').value.trim();
    return clientType === 'reader' ? 'reader' : 'browser';
  };

  const fetchToken = async () => {
    const params = new URLSearchParams({
      subject: subjectForConnect(),
      role: roleForConnect(),
      client_id: byId('clientId').value.trim(),
      reader_id: byId('readerId') ? byId('readerId').value.trim() : '',
      label: byId('label').value.trim(),
    });
    const response = await fetch(`/api/test-token?${params.toString()}`);
    const payload = await response.json().catch(() => ({}));
    if (!response.ok || payload.ok === false) {
      throw new Error(payload.error || `Token request failed with ${response.status}`);
    }
    return payload.token;
  };

  byId('connect').onclick = async () => {
    if (ws && ws.readyState === WebSocket.OPEN) {
      setStatus('Already connected');
      return;
    }
    const normalizedURL = normalizeWSURL(byId('wsUrl').value);
    byId('wsUrl').value = normalizedURL;
    try {
      authToken = await fetchToken();
    } catch (error) {
      setStatus(`Token error: ${error.message}`);
      return;
    }
    const url = new URL(normalizedURL);
    url.searchParams.set('token', authToken);
    setStatus(`Connecting to ${url.toString()}`);
    ws = new WebSocket(url.toString());
    ws.onopen = () => {
      setStatus(`Connected to ${normalizedURL}`);
      send({
        type: 'hello',
        request_id: makeRequestId(),
        payload: {
          client_type: byId('clientType').value.trim(),
          client_id: byId('clientId').value.trim(),
          label: byId('label').value.trim(),
          reader_id: byId('readerId') ? byId('readerId').value.trim() : '',
        },
      });
    };
    ws.onmessage = (event) => {
      log(`RX ${event.data}`);
      try {
        const msg = JSON.parse(event.data);
        if (msg.type === 'hello_ack') {
          myConnectionId = msg.payload.connection_id || '';
          byId('myConnectionId').value = myConnectionId;
          setStatus(`Registered as ${myConnectionId}`);
        } else if (msg.type === 'reply' || msg.type === 'command_ack' || msg.type === 'error') {
          setReaderResponse(msg);
        }
      } catch (_) {}
    };
    ws.onerror = (event) => {
      console.error('WS error', event);
      setStatus(`WS error for ${normalizedURL}. Check /healthz, scheme ws/wss, host, port, and /ws path.`);
    };
    ws.onclose = (event) => {
      setStatus(`Disconnected code=${event.code} clean=${event.wasClean} reason=${event.reason || '-'}`);
    };
  };

  byId('disconnect').onclick = () => {
    if (ws) {
      ws.close();
    }
  };

  byId('refreshConnections').onclick = async () => {
    try {
      const res = await fetch('/api/connections');
      const body = await res.json();
      byId('connections').value = JSON.stringify(body, null, 2);
      log(`HTTP /api/connections -> ${body.count} connections`);
    } catch (error) {
      setStatus(`Connections fetch failed: ${error.message}`);
    }
  };

  byId('sendPing').onclick = () => {
    send({ type: 'ping', request_id: makeRequestId() });
  };

  byId('sendToAll').onclick = () => {
    send({
      type: 'command',
      request_id: makeRequestId(),
      broadcast: true,
      payload: {
        text: byId('message').value,
        from_page: byId('label').value.trim(),
      },
    });
  };

  byId('sendToOne').onclick = () => {
    send({
      type: 'command',
      request_id: makeRequestId(),
      target: {
        mode: 'connection',
        connection_id: byId('targetConnectionId').value.trim(),
      },
      payload: {
        text: byId('message').value,
        from_page: byId('label').value.trim(),
      },
    });
  };

  byId('sendToReaders').onclick = () => {
    send({
      type: 'command',
      request_id: makeRequestId(),
      target: {
        mode: 'client_type',
        client_type: 'reader',
      },
      payload: {
        text: byId('message').value,
        from_page: byId('label').value.trim(),
      },
    });
  };

  const readerCommandSelect = byId('readerCommand');
  if (readerCommandSelect) {
    Object.keys(readerSamples).forEach((commandName) => {
      const option = document.createElement('option');
      option.value = commandName;
      option.textContent = commandName;
      readerCommandSelect.appendChild(option);
    });
    readerCommandSelect.onchange = () => loadReaderSample();
    loadReaderSample('reader.status');
  }

  const loadReaderSampleBtn = byId('loadReaderSample');
  if (loadReaderSampleBtn) {
    loadReaderSampleBtn.onclick = () => loadReaderSample();
  }

  const sendReaderCommandBtn = byId('sendReaderCommand');
  if (sendReaderCommandBtn) {
    sendReaderCommandBtn.onclick = () => sendReaderCommand();
  }

  document.querySelectorAll('.reader-sample-btn').forEach((button) => {
    button.onclick = () => {
      const commandName = button.getAttribute('data-command');
      loadReaderSample(commandName);
    };
  });
}
