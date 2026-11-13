// Controlled subprocess fixture; never invokes Gator accounts or external services.
import readline from 'node:readline';
const state = { conversation_id: 'fixture', source: '/fixture', title: 'Fixture', mode: 'chat', messages: [], running: false, queued: 0 };
process.stdout.write(JSON.stringify({version:1,type:'state',state})+'\n');
if(process.env.GATOR_FIXTURE_NO_READ==='1'){
 process.stdin.pause(); process.on('SIGTERM',()=>{}); setInterval(()=>{},1000);
}else readline.createInterface({input:process.stdin}).on('line', line => {
 const frame=JSON.parse(line);
 if(frame.type==='exit')process.exit(0);
 if(frame.text==='/cancel')process.exit(7);
 if(frame.nonce)process.stdout.write(JSON.stringify({version:1,type:'action_applied',nonce:frame.nonce})+'\n');
});
