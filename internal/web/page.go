package web

const pageHTML = `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Ask my digital twin</title>
<style>
 body{font:16px/1.5 system-ui;max-width:640px;margin:0 auto;padding:16px}
 #log div{margin:8px 0;padding:8px 12px;border-radius:10px;white-space:pre-wrap}
 .user{background:#e8f0fe;margin-left:15%}.bot{background:#f1f1f1;margin-right:15%}
 form{display:flex;gap:8px;position:sticky;bottom:0;background:#fff;padding:8px 0}
 input{flex:1;padding:10px;font:inherit}button{padding:10px 16px;font:inherit}
</style></head><body>
<h1>Ask my digital twin</h1>
<p>An AI that answers questions about me, using only my CV and notes.</p>
<div id="log"></div>
<form id="f"><input id="q" maxlength="500" placeholder="e.g. What have you built with Go?" autocomplete="off" required>
<button>Ask</button></form>
<script>
const log=document.getElementById('log'),f=document.getElementById('f'),q=document.getElementById('q');
let history=[];
function add(cls,text){const d=document.createElement('div');d.className=cls;d.textContent=text;log.appendChild(d);d.scrollIntoView();return d}
f.onsubmit=async e=>{
  e.preventDefault();
  const text=q.value.trim();if(!text)return;q.value='';
  history.push({role:'user',content:text});add('user',text);
  const out=add('bot','…');
  try{
    const r=await fetch('/chat',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({history})});
    if(!r.ok){out.textContent=await r.text();history.pop();return}
    const rd=r.body.getReader(),dec=new TextDecoder();let acc='';out.textContent='';
    for(;;){const {done,value}=await rd.read();if(done)break;acc+=dec.decode(value,{stream:true});out.textContent=acc;out.scrollIntoView()}
    history.push({role:'assistant',content:acc});
  }catch(err){out.textContent='Something went wrong.';history.pop()}
};
</script></body></html>`
