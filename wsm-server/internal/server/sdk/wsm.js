/* WiseMED WSM browser SDK v1. No dependencies. JWTs stay in memory. */
(function (root) {
  'use strict';
  const error = (code, message, details) => Object.assign(new Error(message), {code, ...details});
  function claims(token) {
    try {
      const part = token.split('.')[1].replace(/-/g, '+').replace(/_/g, '/');
      return JSON.parse(new TextDecoder().decode(Uint8Array.from(atob(part.padEnd(Math.ceil(part.length / 4) * 4, '=')), c => c.charCodeAt(0))));
    } catch { throw error('invalid_token', 'JWT format invalid. Obtain a token from the WiseMED backend.'); }
  }
  function socketURL(value) {
    const url = new URL(value);
    if (url.protocol === 'https:') url.protocol = 'wss:';
    if (url.protocol !== 'wss:' && !(url.protocol === 'ws:' && ['localhost','127.0.0.1','[::1]'].includes(url.hostname))) throw error('invalid_url', 'Use wss:// (ws:// only on loopback).');
    if (url.username || url.password || url.search || url.hash) throw error('invalid_url', 'Credentials, query and fragment are not allowed in the WSM URL.');
    if (url.pathname === '/') url.pathname = '/ws';
    return url.href;
  }
  class Client {
    constructor(url, token, options = {}) {
      this.url = socketURL(url);
      this.options = {timeout:35000, connectTimeout:10000, reconnectDelay:30000, autoReconnect:true, ...options};
      this._token = token; this._pending = new Map(); this._generation = 0; this._stopped = false;
      this.isConnected = false; this.connectionID = ''; this.clients = [];
      this.onConnect = null; this.onDisconnect = null; this.onError = null; this.onTokenRequired = null;
      this.onEvent = null; this.onCommand = null; this.onReply = null; this.onPresence = null; this.onMessage = null;
      this.ready = this.connect();
      this.ready.catch(() => {}); // Also available to callers via await ready / onError.
    }
    _emit(name, value) {
      if (typeof this[name] !== 'function') return;
      try { Promise.resolve(this[name](value)).catch(e => { if(name !== 'onError') this._emit('onError', e); }); }
      catch(e) { if(name !== 'onError') this._emit('onError', e); }
    }
    connect(token) {
      if (token !== undefined) this._token = token;
      if (this.isConnected) return Promise.resolve(this);
      if (this._connecting) return this.ready;
      this._stopped = false; clearTimeout(this._retry);
      this._connecting = true;
      const generation = ++this._generation;
      this.ready = new Promise((resolve, reject) => {
        this._resolveConnect = resolve; this._rejectConnect = reject;
      });
      this.ready.catch(() => {});
      Promise.resolve().then(async () => {
        let jwt = this._token;
        if (typeof this.options.tokenProvider === 'function') jwt = await this.options.tokenProvider();
        if (generation !== this._generation || this._stopped) return;
        const identity = claims(jwt); // Used only for hello; WSM verifies signature and permissions.
        if (identity.role !== 'browser' || !identity.client_id || !identity.exp) throw error('invalid_token', 'SDK requires a browser JWT with client_id and exp.');
        if (identity.exp * 1000 <= Date.now()) throw error('token_expired', 'Obtain a fresh JWT from WiseMED.');
        this._token = jwt; this._expires = identity.exp * 1000;
        const ws = new WebSocket(this.url, ['wsm.v1', 'wsm.jwt.' + jwt]); this._ws = ws;
        this._helloTimer = setTimeout(() => {
          if (generation === this._generation) { this._fail(error('connect_timeout','WSM hello timed out.')); ws.close(); }
        }, this.options.connectTimeout);
        ws.onopen = () => {
          if (generation !== this._generation) return;
          ws.send(JSON.stringify({type:'hello',payload:{client_type:'browser',client_id:identity.client_id}}));
        };
        ws.onmessage = event => {
          if (generation !== this._generation) return;
          try { this._receive(JSON.parse(event.data)); }
          catch(e) { this._emit('onError', e); }
        };
        ws.onerror = () => { if(generation === this._generation) this._emit('onError',error('websocket_error','WebSocket failed. Check WSM logs for JWT, Origin and proxy diagnostics.')); };
        ws.onclose = event => {
          if (generation !== this._generation) return;
          this._fail(error('disconnected','WSM disconnected.',{closeCode:event.code}));
          this._emit('onDisconnect',{code:event.code,reason:event.reason,wasClean:event.wasClean});
          this._scheduleReconnect();
        };
      }).catch(e => {
        if(generation !== this._generation) return;
        this._fail(e); this._emit('onError',e); this._scheduleReconnect();
      });
      return this.ready;
    }
    _scheduleReconnect() {
      if (this._stopped || !this.options.autoReconnect) return;
      if (typeof this.options.tokenProvider !== 'function' && (!this._expires || this._expires <= Date.now())) {
        this._emit('onTokenRequired',{}); return;
      }
      clearTimeout(this._retry);
      this._retry = setTimeout(() => this.connect(), this.options.reconnectDelay);
    }
    _fail(e) {
      clearTimeout(this._helloTimer); this.isConnected = false; this._connecting = false;
      this.connectionID = ''; this.clients = [];
      this._rejectConnect?.(e); this._resolveConnect = this._rejectConnect = null;
      for (const pending of this._pending.values()) { clearTimeout(pending.timer); pending.reject(e); }
      this._pending.clear();
    }
    disconnect() {
      this._stopped = true; ++this._generation; clearTimeout(this._retry);
      const active = this.isConnected || this._connecting;
      this._fail(error('disconnected','Disconnected by application.')); this._ws?.close();
      if(active) this._emit('onDisconnect',{code:1000,reason:'manual',wasClean:true});
    }
    reconnect(token) { this.disconnect(); return this.connect(token); }
    _receive(message) {
      const p = message.payload || {};
      if (message.type === 'hello_ack' && this._connecting) {
        clearTimeout(this._helloTimer); this._connecting = false; this.isConnected = true;
        this.connectionID = p.connection_id; this._resolveConnect?.(this);
        this._resolveConnect = this._rejectConnect = null; this._emit('onConnect',p); return;
      }
      const pending = this._pending.get(message.correlation_id || message.request_id);
      if (pending) {
        const sender = message.connection_id || p.sender_connection_id;
        const serverMessage = !sender;
        let failure;
        let done = false;
        if (message.type === 'error' && serverMessage) { failure=error(p.code||'wsm_error',p.message||'WSM request failed.'); done=true; }
        else if (message.type === 'command_ack' && serverMessage && pending.expected === 'reply') {
          if(p.recipients === 0) { failure=error('no_recipient','Target offline or unavailable.'); done=true; }
        } else if (message.type === pending.expected && (pending.peer ? sender === pending.peer : serverMessage)) {
          done=true;
          if(p.ok === false) failure=error('operation_failed',typeof p.error === 'string'?p.error:(p.error?.message||'Operation refused.'),{response:p});
        }
        if(done) {
          clearTimeout(pending.timer); this._pending.delete(pending.id);
          if(failure) pending.reject(failure); else pending.resolve(message.type === 'command_ack' ? {...p,requestID:pending.id} : p);
        }
      }
      this._emit('onMessage',message);
      const callback = {event:'onEvent',command:'onCommand',reply:'onReply',presence:'onPresence'}[message.type];
      if(callback) this._emit(callback,message);
    }
    async _request(type, payload = {}, target, expected = 'reply', peer, timeout, correlationID) {
      await this.ready;
      if(!this.isConnected) throw error('disconnected','WSM is not connected.');
      if(this._pending.size >= 256) throw error('busy','Too many outstanding requests.');
      const id = crypto.randomUUID();
      return new Promise((resolve,reject) => {
        const timer = setTimeout(() => {this._pending.delete(id);reject(error('timeout','Request timed out; execution may have occurred. Do not automatically replay.',{requestID:id}));},timeout ?? this.options.timeout);
        this._pending.set(id,{id,resolve,reject,timer,expected,peer});
        try {this._ws.send(JSON.stringify({type,request_id:id,...(correlationID?{correlation_id:correlationID}:{}),...(target?{target}:{}),payload}));}
        catch(e) {clearTimeout(timer);this._pending.delete(id);reject(e);}
      });
    }
    ping() { return this._request('ping',{},undefined,'pong'); }
    serverCommand(command,args = {}) {return this._request('command',{command,args},{mode:'server'});}
    serverStatus() {return this.serverCommand('server.status');}
    equipmentStatus(ids) {return this.serverCommand('equipment.status',Array.isArray(ids)?{equipment_ids:ids.map(String)}:{equipment_id:String(ids)});}
    controlTicket(equipmentID) {return this.serverCommand('control.ticket',{equipment_id:String(equipmentID)});}
    async listClients() {
      const data = await this._request('list_connections',{},undefined,'connections');
      this.clients = (data.connections || []).map(c => ({...c,connectionID:c.id,clientID:c.client_id,clientType:c.client_type,equipmentID:c.equipment_id,readerID:c.reader_id}));
      return this.clients;
    }
    async getClientsByType(type) {
      if(typeof type !== 'string') throw error('invalid_type','Client type must be a string.');
      const matches = (await this.listClients()).filter(c => c.clientType === type);
      return matches.length ? matches : null;
    }
    async _target(client) {
      if(Array.isArray(client)) throw error('ambiguous_target','Select one client, or pass the array to callAPI for explicit fan-out.');
      if(client && typeof client === 'object' && (client.connectionID || client.id)) return {mode:'connection',connection_id:client.connectionID||client.id};
      if(typeof client !== 'string') throw error('invalid_target','Use a client object, clientID or connectionID.');
      const clients = await this.listClients();
      const matches = clients.filter(c => c.connectionID === client || c.clientID === client);
      if(matches.length !== 1) throw error(matches.length?'ambiguous_target':'no_recipient',matches.length?'Multiple sessions use this clientID; select a client object.':'Client not connected.');
      return {mode:'connection',connection_id:matches[0].connectionID};
    }
    async command(client,command,args = {},options = {}) {
      const target = await this._target(client);
      return this._request('command',{command,args},target,'reply',target.connection_id,options.timeout);
    }
    callAPI(client,path,params = {},onFail,onSuccess) {
      const promise = (async () => {
        if(Array.isArray(client)) return Promise.all(client.map(c => this.callAPI(c,path,params)));
        const opts = params || {};
        const method = (opts.method || 'GET').toUpperCase();
        if(!['GET','HEAD','POST','PUT','PATCH','DELETE'].includes(method)) throw error('invalid_method','Unsupported HTTP method.');
        if(typeof path !== 'string' || !path.startsWith('/') || path.startsWith('//')) throw error('invalid_path','Use a relative data API path.');
        if(opts.query) { const query = new URLSearchParams(opts.query).toString(); if(query) path += (path.includes('?')?'&':'?')+query; }
        const args = {method,path};
        for(const field of ['body','body_base64','content_type']) if(opts[field] !== undefined) args[field]=opts[field];
        const response = await this.command(client,'api.request',args,{timeout:opts.timeout});
        if(response.kind !== 'api.response' || !Number.isInteger(response.status)) throw error('invalid_response','Expected api.response.',{response});
        if(response.status < 200 || response.status >= 300) throw error('http_error',`Remote API returned HTTP ${response.status}.`,{status:response.status,response});
        if(response.body_base64 !== undefined) response.bytes=Uint8Array.from(atob(response.body_base64),c=>c.charCodeAt(0));
        return response;
      })();
      if(onSuccess || onFail) promise.then(value => {if(onSuccess) onSuccess(value);},e => {if(onFail) onFail(e);else this._emit('onError',e);}).catch(e => this._emit('onError',e));
      return promise;
    }
    subscribe(topic) {return this._request('subscribe',{topic},undefined,'subscribe_ack');}
    unsubscribe(topic) {return this._request('unsubscribe',{topic},undefined,'unsubscribe_ack');}
    // Advanced routing returns queue ACK only; peer replies arrive via onReply.
    send(type,target,payload = {}) {
      if(target?.mode === 'server') return Promise.reject(error('invalid_target','Use serverCommand() for server operations.'));
      if(!['command','event'].includes(type)) return Promise.reject(error('invalid_type','send supports command and event. Use reply() for responses.'));
      return this._request(type,payload,target,'command_ack');
    }
    reply(message,payload) {
      const peer = message.connection_id || message.payload?.sender_connection_id;
      if(!peer || !message.request_id) return Promise.reject(error('invalid_request','Expected a received command.'));
      return this._request('reply',payload,{mode:'connection',connection_id:peer},'command_ack',undefined,undefined,message.request_id);
    }
  }
  function WSMServer(url,token,options) {return new Client(url,token,options);}
  WSMServer.Client = Client;
  if(typeof module !== 'undefined' && module.exports) module.exports = WSMServer;
  else root.WSMServer = WSMServer;
})(typeof globalThis !== 'undefined' ? globalThis : window);
