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
