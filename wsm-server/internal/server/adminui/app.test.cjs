const {test}=require('node:test');const assert=require('node:assert/strict');const {parseCommand,appendOutput}=require('./app.js');
test('allowlisted commands and optional reader binding',()=>{assert.deepEqual(parseCommand('add 42'),{command:'add',equipment_id:'42'});assert.deepEqual(parseCommand(' add 42 reader-1 '),{command:'add',equipment_id:'42',reader_id:'reader-1'});for(const command of ['help','devices','list','clear','logout'])assert.deepEqual(parseCommand(command),{command});});
test('shell syntax and unexpected arguments are rejected',()=>{for(const text of ['rm -rf /','devices; whoami','add 42 $(whoami)','add 42 reader >x','add','devices --all','add <script>','add ../secret','add '+'x'.repeat(129),''])assert.throws(()=>parseCommand(text));});
test('output never interprets attacker supplied markup',()=>{const host={children:[],append(value){this.children.push(value)}};const doc={createElement:()=>({})};appendOutput(doc,host,'<img src=x onerror=alert(1)>');assert.equal(host.children[0].textContent,'<img src=x onerror=alert(1)>');assert.equal(Object.hasOwn(host.children[0],'innerHTML'),false);});

test('key requires one safe equipment ID',()=>{assert.deepEqual(parseCommand('key 42'),{command:'key',equipment_id:'42'});assert.throws(()=>parseCommand('key'));assert.throws(()=>parseCommand('key 42 extra'));});

test('edit and delete use equipment IDs and explicit reader removal',()=>{
 assert.deepEqual(parseCommand('edit 42 horiba-yumizen-h500-reader-v3'),{command:'edit',equipment_id:'42',reader_id:'horiba-yumizen-h500-reader-v3'});
 assert.deepEqual(parseCommand('edit 42 --remove-reader'),{command:'edit',equipment_id:'42',reader_id:''});
 assert.deepEqual(parseCommand('delete 42'),{command:'delete',equipment_id:'42'});
 for(const input of ['edit 42','edit 42 r extra','edit 42 ../bad','edit 42 --unknown','delete','delete 42 extra'])assert.throws(()=>parseCommand(input));
});

test('permission editor accepts bounded numeric selections and explicit save',()=>{
 assert.deepEqual(parseCommand('rights 1'),{command:'rights',equipment_id:'1'});
 assert.deepEqual(parseCommand('toggle 1 7 7'),{command:'toggle',numbers:[1,7]});
 for(const command of ['save','cancel'])assert.deepEqual(parseCommand(command),{command});
 for(const command of ['toggle','toggle 0','toggle -1','toggle 1.5','toggle 9007199254740992','rights','save 1'])assert.throws(()=>parseCommand(command));
});

test('info accepts one or multiple unique permission numbers',()=>{
 assert.deepEqual(parseCommand('info 3'),{command:'info',numbers:[3]});
 assert.deepEqual(parseCommand('info 3 7 3'),{command:'info',numbers:[3,7]});
 for(const input of ['info','info 0','info -1','info 1.5','info api:invoke','info 9007199254740992'])assert.throws(()=>parseCommand(input));
});
test('info displays only requested details without changing the catalog',()=>{
 const {permissionInfo}=require('./app.js');
 const catalog=[{id:'route:reply',description:'Titlu',info:'Detalii',example:'Exemplul'},{id:'api:invoke',description:'API',info:'Detalii API',example:'Exemplu API'}];
 const before=JSON.stringify(catalog);
 assert.equal(permissionInfo(catalog,[2]),'2. api:invoke — API\n   Info: Detalii API\n   Exemplu: Exemplu API');
 assert.match(permissionInfo(catalog,[2,1]),/Exemplu API\n\n1\. route:reply/);
 assert.throws(()=>permissionInfo(catalog,[1,3]),/inexistent/);
 assert.equal(JSON.stringify(catalog),before);
});

test('command history persists across runs and restores the unfinished input',()=>{
 const {CommandHistory,historyKey}=require('./app.js');const saved=new Map();
 const storage={getItem:key=>saved.get(key),setItem:(key,value)=>saved.set(key,value)};
 const key=historyKey({tenant_id:'clinic',username:'admin'});
 const first=new CommandHistory(storage,key);first.add('rights 18');first.add('info 3 7');
 const next=new CommandHistory(storage,key);
 assert.equal(next.move(-1,'add 19'),'info 3 7');
 assert.equal(next.move(-1,'info 3 7'),'rights 18');
 assert.equal(next.move(-1,'rights 18'),'rights 18');
 assert.equal(next.move(1,'rights 18'),'info 3 7');
 assert.equal(next.move(1,'info 3 7'),'add 19');
 next.add('clear');next.add('history');next.add('logout');
 assert.deepEqual(new CommandHistory(storage,key).entries,['rights 18','info 3 7','clear','history','logout']);
 assert.deepEqual(new CommandHistory(storage,historyKey({tenant_id:'other',username:'admin'})).entries,[]);
 assert.deepEqual(new CommandHistory(storage,historyKey({tenant_id:'clinic',username:'other'})).entries,[]);
 assert.deepEqual(parseCommand('history'),{command:'history'});assert.throws(()=>parseCommand('history 1'));
});
test('history rejects invalid commands and survives unavailable or corrupt storage',()=>{
 const {CommandHistory}=require('./app.js');
 const blocked=new CommandHistory(undefined,'key');blocked.add('list');
 assert.equal(blocked.persistent,false);assert.equal(blocked.move(-1,''),'list');
 assert.throws(()=>blocked.add('password secret'));assert.deepEqual(blocked.entries,['list']);
 const corrupted=new CommandHistory({getItem:()=>'{bad json'},'key');assert.deepEqual(corrupted.entries,[]);
 const filtered=new CommandHistory({getItem:()=>JSON.stringify(['help',null,'unrecognized secret'])},'key');assert.deepEqual(filtered.entries,['help']);
});
