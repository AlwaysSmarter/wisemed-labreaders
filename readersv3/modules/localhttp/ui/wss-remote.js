/* Shared UI data transport. Remote mode sends JSON API commands only; assets stay local. */
(function (root) {
  'use strict';
  function createTransport(options) {
    const pending = new Map(), waiters = new Set();
    let socket, ready = false, authenticatedOnce = false, sequence = 0, generation = 0;
    const timeout = options.timeout || 35000;
    function error(message) { return new Error(message); }
    function rejectAll(reason, includeWaiters = true) {
      for (const entry of [...pending.values(), ...(includeWaiters ? waiters : [])]) entry.finish(reason);
    }
    function connect(ticket) {
      if (!ticket || !/^[A-Za-z0-9_-]+$/.test(ticket)) throw error('Invalid WSS ticket');
      const current = ++generation;
      ready = false;
      if (socket) { socket.close(); rejectAll(error('WSS connection replaced'), authenticatedOnce); }
      const ws = socket = new options.WebSocket(options.url, ['wsm.v1', 'wsm.ticket.' + ticket]);
      const handshake = setTimeout(() => { if (current === generation && !ready) ws.close(); }, timeout);
      ws.onopen = () => ws.send(JSON.stringify({type:'hello',payload:{client_type:'browser',client_id:'control'}}));
      ws.onmessage = event => {
        if (current !== generation) return;
        let message; try { message = JSON.parse(event.data); } catch (_) { return; }
        if (message.type === 'hello_ack') {
          clearTimeout(handshake); ready = true; authenticatedOnce = true;
          for (const waiter of [...waiters]) waiter.finish();
          options.onState?.('connected'); return;
        }
        const entry = pending.get(message.correlation_id || message.request_id);
        if (!entry) return;
        if (message.type === 'command_ack') {
          if (message.payload?.recipients === 0) entry.finish(error('Equipment is offline or access denied'));
          return; // A routing acknowledgement is never the API response.
        }
        if (message.type === 'error') entry.finish(error(message.payload?.message || 'WSS command failed'));
        if (message.type === 'reply') entry.finish(null, message.payload);
      };
      ws.onerror = () => { if (current === generation) options.onState?.('error'); };
      ws.onclose = () => {
        clearTimeout(handshake);
        if (current !== generation) return;
        ready = false; rejectAll(error('WSS connection closed'), authenticatedOnce); options.onState?.('disconnected');
      };
      options.onState?.('connecting');
    }
    function wait(signal, register, awaitingInitialAuth = false) {
      return new Promise((resolve, reject) => {
        let timer;
        const abort = () => entry.finish(signal.reason || new DOMException('Aborted', 'AbortError'));
        const entry = {finish(reason, result) {
          clearTimeout(timer); signal?.removeEventListener('abort', abort);
          waiters.delete(entry); if (entry.id) pending.delete(entry.id);
          reason ? reject(reason) : resolve(result);
        }};
        if (signal?.aborted) { abort(); return; }
        signal?.addEventListener('abort', abort, {once:true});
        // Startup waits for the user/opener to authorize. No API command exists yet.
        // Once authenticated, both reconnect waits and actual commands are bounded.
        if (!awaitingInitialAuth) timer = setTimeout(() => entry.finish(error('WSS request timed out')), timeout);
        register(entry);
      });
    }
    async function fetchAPI(input, init) {
      const request = new Request(input, init);
      const url = new URL(request.url);
      const bytes = new Uint8Array(await request.arrayBuffer());
      if (bytes.length > 4 * 1024 * 1024) throw error('WSS request body exceeds 4 MiB');
      if (!ready) await wait(request.signal, entry => waiters.add(entry), !authenticatedOnce);
      if (request.signal.aborted) throw request.signal.reason;
      const args = {method:request.method, path:url.pathname + url.search, content_type:request.headers.get('content-type') || 'application/json'};
      if (bytes.length) {
        let binary = ''; for (let i = 0; i < bytes.length; i += 8192) binary += String.fromCharCode(...bytes.subarray(i, i + 8192));
        args.body_base64 = btoa(binary);
      }
      const payload = await wait(request.signal, entry => {
        entry.id = 'control-' + (++sequence); pending.set(entry.id, entry);
        try { socket.send(JSON.stringify({type:'command',request_id:entry.id,target:{mode:'equipment',equipment_id:options.equipmentID},payload:{command:'api.request',args}})); }
        catch (cause) { entry.finish(cause); }
      });
      if (payload?.kind !== 'api.response') throw error(payload?.error?.message || payload?.error || 'Invalid WSS API response');
      const headers = new Headers(payload.headers || {});
      if (payload.content_type) headers.set('content-type', payload.content_type);
      if ((headers.get('content-type') || '').toLowerCase().includes('text/html')) throw error('HTML responses are not allowed through WSS');
      let body = payload.body_base64 != null ? Uint8Array.from(atob(payload.body_base64), c => c.charCodeAt(0)) : (Object.prototype.hasOwnProperty.call(payload, 'body') ? JSON.stringify(payload.body) : '');
      if (request.method === 'HEAD' || [204,205,304].includes(payload.status)) body = null;
      return new Response(body, {status:payload.status, headers});
    }
    return {connect, fetch:fetchAPI, close() { generation++; ready = false; socket?.close(); rejectAll(error('WSS closed')); }, get connected() { return ready; }};
  }
  if (typeof module !== 'undefined' && module.exports) { module.exports = {createTransport}; return; }
  if (!root.location.pathname.startsWith('/control/')) return;
  const params = new URLSearchParams(root.location.search), equipmentID = params.get('equipment_id');
  const nativeFetch = root.fetch.bind(root);
  let openerOrigin = '';
  try {
    const candidate = params.get('opener_origin') || (document.referrer && new URL(document.referrer).origin);
    const parsed = new URL(candidate);
    if (['http:', 'https:'].includes(parsed.protocol) && parsed.origin === candidate) openerOrigin = candidate;
  } catch (_) {}
  let banner, status, form, retryAt = 0, closed = false;
  function tellOpener(type) { if (!closed && root.opener && openerOrigin) root.opener.postMessage({type, equipment_id:equipmentID}, openerOrigin); }
  function show(text) { if (status) status.textContent = 'Control la distanță · ' + equipmentID + ' · ' + text; }
  const transport = createTransport({WebSocket:root.WebSocket, url:(location.protocol === 'https:' ? 'wss://' : 'ws://') + location.host + '/ws',equipmentID,
    onState(state) {
      show({connected:'WSS conectat',connecting:'Conectare WSS…',error:'Eroare WSS',disconnected:'WSS deconectat'}[state]);
      if (state === 'connected') { retryAt = 0; if(form) form.hidden = true; }
      if (state === 'disconnected') { retryAt = Date.now() + 30000; if(form) form.hidden = false; }
    }});
  root.WSM_REMOTE = {equipmentID, route:() => new URLSearchParams(location.search).get('route') || '/', reconnect:() => tellOpener('wsm-control-refresh'), close:() => { closed = true; retryAt = 0; transport.close(); show('Control închis'); }};
  // Keep reloads and copied links on the central static UI while preserving the logical view.
  for (const method of ['pushState','replaceState']) {
    const original = root.history[method].bind(root.history);
    root.history[method] = function (state, title, path) {
      if (path != null) {
        const destination = new URL(path, location.href);
        if (destination.origin === location.origin && !destination.pathname.startsWith('/control/')) {
          const remote = new URL(location.href); remote.pathname = '/control/'; remote.searchParams.set('route', destination.pathname + destination.search); path = remote.href;
        }
      }
      return original(state, title, path);
    };
  }
  root.fetch = function (input, init) {
    const url = new URL(input instanceof Request ? input.url : input, location.href);
    if (url.origin === location.origin && (url.pathname.startsWith('/api/') || url.pathname === '/barcode/print')) {
      if (!equipmentID) return Promise.reject(new Error('Missing equipment_id'));
      return transport.fetch(input instanceof Request ? input : url.href, init);
    }
    return nativeFetch(input, init);
  };
  root.addEventListener('message', event => {
    if (closed || !root.opener || event.source !== root.opener || !openerOrigin || event.origin !== openerOrigin || event.data?.type !== 'wsm-control-auth') return;
    try { transport.connect(event.data.ticket); } catch (error) { show(error.message); }
  });
  function mount() {
    banner = document.createElement('div'); banner.setAttribute('role','status');
    banner.style.cssText = 'position:sticky;top:0;z-index:9999;background:#143e60;color:white;padding:10px 16px;font:14px system-ui;display:flex;gap:12px;align-items:center;flex-wrap:wrap';
    status = document.createElement('span'); banner.append(status);
    const button = document.createElement('button'); button.textContent = 'Reconectează controlul'; button.onclick = () => { retryAt = Date.now() + 30000; tellOpener('wsm-control-refresh'); }; banner.append(button);
    form = document.createElement('form');
    const ticket = document.createElement('input'); ticket.type = 'password'; ticket.placeholder = 'Tichet WSS de unică folosință'; ticket.autocomplete = 'off'; ticket.setAttribute('aria-label', ticket.placeholder);
    const submit = document.createElement('button'); submit.textContent = 'Conectează'; form.append(ticket, submit);
    form.onsubmit = event => { event.preventDefault(); const value = ticket.value.trim(); ticket.value = ''; try { transport.connect(value); } catch (error) { show(error.message); } };
    banner.append(form); document.body.prepend(banner); show('Aștept autorizarea conexiunii'); tellOpener('wsm-control-ready');
    setInterval(() => {
      if (!retryAt) return;
      const seconds = Math.max(0, Math.ceil((retryAt - Date.now()) / 1000)); show('WSS deconectat · reîncercare în ' + seconds + 's');
      if (!seconds) { retryAt = Date.now() + 30000; tellOpener('wsm-control-refresh'); }
    }, 1000);
  }
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', mount, {once:true}); else mount();
})(typeof window !== 'undefined' ? window : globalThis);
