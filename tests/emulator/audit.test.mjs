import {test} from 'node:test';
import assert from 'node:assert/strict';
import {spawnSync} from 'node:child_process';

const endpoint='http://127.0.0.1:8080/v1/projects/demo-fql/databases/(default)/documents/users';
test('the Go audit CLI reports malformed emulator data without values or repairs', async()=>{
  const reset=await fetch('http://127.0.0.1:8080/emulator/v1/projects/demo-fql/databases/(default)/documents',{method:'DELETE'});
  assert.equal(reset.status,200);
  const fields={name:{integerValue:'42'},active:{booleanValue:true},status:{stringValue:'active'},bio:{stringValue:'PRIVATE_TEST_CONTENT'}};
  const seeded=await fetch(endpoint+'/audit-fixture',{method:'PATCH',headers:{Authorization:'Bearer owner','Content-Type':'application/json'},body:JSON.stringify({fields})});
  assert.equal(seeded.status,200);
  const result=spawnSync('go',['run','./cmd/fql','audit','--project','examples/flutter','--firebase-project','demo-fql','--emulator','127.0.0.1:8080','--model','User','--json'],{encoding:'utf8'});
  assert.equal(result.status,1); // go run translates the CLI's exit 3 to process exit 1.
  const report=JSON.parse(result.stdout);
  assert.equal(report.scanned,1);assert.equal(report.complete,true);assert.equal(report.violations[0].issues[0].field,'name');
  assert.equal(result.stdout.includes('PRIVATE_TEST_CONTENT'),false);
  const after=await fetch(endpoint+'/audit-fixture',{headers:{Authorization:'Bearer owner'}});
  assert.equal((await after.json()).fields.name.integerValue,'42');
});
