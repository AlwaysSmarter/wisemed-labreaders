const statusBox = document.getElementById('health');
const form = document.getElementById('settings');
const preview = document.getElementById('signature');
let socket;
let sequence = 0;
let pending = false;
function controls(disabled) {
  document.querySelectorAll('[data-action]').forEach(button => { button.disabled = disabled; });
}
function connect() {
  controls(true);
  socket = new WebSocket(`${location.protocol === 'https:' ? 'wss:' : 'ws:'}//${location.host}/ws`);
  socket.onopen = () => { controls(false); statusBox.textContent = 'Conectat. Detectează pad-ul sau începe semnătura.'; };
  socket.onmessage = event => {
    pending = false;
    controls(false);
    const data = JSON.parse(event.data);
    if (data.imageBase64) {
      preview.src = `data:image/png;base64,${data.imageBase64}`;
      preview.hidden = false;
      delete data.imageBase64;
    }
    statusBox.textContent = JSON.stringify(data, null, 2);
  };
  socket.onerror = () => { statusBox.textContent = 'Conexiunea WSS a eșuat. Verifică serviciul și certificatul localhost.'; };
  socket.onclose = () => { pending = false; controls(false); statusBox.textContent = 'Sesiune închisă. Apasă un buton pentru reconectare, apoi repetă comanda.'; };
}
document.getElementById('controls').onclick = event => {
  const action = event.target.dataset.action;
  if (!action || pending) return;
  if (!socket || socket.readyState !== WebSocket.OPEN) { connect(); return; }
  if (action === 'start' || action === 'retry' || action === 'cancel') { preview.hidden = true; preview.removeAttribute('src'); }
  pending = true;
  controls(true);
  socket.send(JSON.stringify({id: String(++sequence), action}));
};
form.onsubmit = async event => {
  event.preventDefault();
  try {
    const response = await fetch('/api/esignature/settings', {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify(Object.fromEntries(new FormData(form)))});
    if (!response.ok) throw new Error(await response.text());
    statusBox.textContent = 'Setări salvate.';
  } catch (error) { statusBox.textContent = String(error); }
};
(async () => {
  try {
    const response = await fetch('/api/esignature/settings');
    if (!response.ok) throw new Error(await response.text());
    const settings = await response.json();
    for (const [key,value] of Object.entries(settings)) if(form.elements[key]) form.elements[key].value=value;
  } catch (error) { statusBox.textContent=String(error); }
})();
connect();

async function loadHistory() {
  try {
    for (const [element, path] of [['history', '/api/esignature/jobs'], ['stats', '/api/esignature/stats/daily']]) {
      const response = await fetch(path);
      if (!response.ok) throw new Error(await response.text());
      document.getElementById(element).textContent = JSON.stringify(await response.json(), null, 2);
    }
  } catch (error) { document.getElementById('history').textContent = String(error); }
}
document.getElementById('refresh-history').onclick = loadHistory;
loadHistory();
