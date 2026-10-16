const assert = require('node:assert/strict');
const WSMServer = require('./wsm.js');
const SIUIBridge = require('./siui-bridge.js');
(async () => {
 const wsm = WSMServer(process.env.WSM_TEST_URL,process.env.WSM_TEST_TOKEN,{autoReconnect:false});
 try {
  await wsm.ready;assert.equal(wsm.isConnected,true);
  assert.ok((await wsm.ping()).server_time);
  assert.equal((await wsm.serverStatus()).tenant_id,'a');
  const clients = await wsm.getClientsByType('siui-bridge');assert.equal(clients.length,1);
  assert.equal(await wsm.getClientsByType('nonexistent-utility'),null);
  const bridge=await SIUIBridge.create(wsm);assert.ok(bridge);assert.equal(bridge.isConnected,true);
  assert.equal((await bridge.getStatus()).ok,true);
  const response = await wsm.callAPI(clients[0],'/api/status',{method:'GET'});assert.equal(response.body.ok,true);
  assert.equal((await wsm.callAPI(clients[0].clientID,'/api/status')).status,200);
  assert.equal((await wsm.subscribe('test')).ok,true);assert.equal((await wsm.unsubscribe('test')).ok,true);
 } finally {wsm.disconnect();}
})().catch(e => {console.error(e.message);process.exitCode=1;});
