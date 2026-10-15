Object.assign(menuErrors,{privacy_review_changed:"Os registros mudaram durante a revisão. Feche e revise novamente antes de remover.",privacy_review_failed:"Não foi possível revisar os registros. Tente novamente."});
async function lgpdDelete(customerId){
 const store=currentStoreId,c=await api("customer-privacy-review?customer_id="+customerId);
 if(store!==currentStoreId)return;
 let dialog=$("privacyReviewDialog");
 if(!dialog){dialog=menuNode("dialog","dialog menu-editor-dialog");dialog.id="privacyReviewDialog";dialog.setAttribute("aria-labelledby","privacyReviewTitle");dialog.innerHTML='<div class="dialog-header"><h2 id="privacyReviewTitle"></h2><button type="button" class="icon-button" aria-label="Fechar revisão de privacidade">✕</button></div><div class="dialog-body"></div>';dialog.querySelector("button").onclick=()=>dialog.close();document.body.append(dialog)}
 $("privacyReviewTitle").textContent="Revisar remoção · "+c.name;
 const root=dialog.querySelector(".dialog-body");root.replaceChildren();
 root.append(menuNode("p","menu-hint","A remoção limpa o cadastro, contatos e textos livres vinculados. Os valores, IDs, datas e composição dos pedidos ficam preservados. Motivos de recebimentos e descontos serão substituídos por um marcador de remoção."));
 const list=menuNode("dl","privacy-impact");for(const [key,label] of Object.entries({orders:"Pedidos",items:"Itens de pedido",consents:"Consentimentos",receipts:"Recebimentos/devoluções",checkouts:"Vendas do PDV",tabs:"Comandas vinculadas"})){list.append(menuNode("dt","",label),menuNode("dd","",c.counts[key]))}root.append(list);
 root.append(menuNode("p","notice","Rótulos de comandas vinculadas serão substituídos pelo número da comanda, inclusive quando compartilhadas com outros clientes. Backups e exportações anteriores não são alterados. Campos sem vínculo ao cliente exigem revisão manual. A retenção só é executada se ativada na política da loja. Não há garantia de anonimato."));
 const form=menuNode("form","menu-admin-form");root.append(form);form.id="privacyRemovalForm";
 const confirm=menuCheck(form,"Confirmo a remoção de identificação e textos livres dos registros listados",false,"privacy_confirm");confirm.required=true;
 const error=menuNode("p","pos-inline-error hidden");error.setAttribute("role","alert");const save=menuNode("button","btn-cancel","Remover identificação e textos vinculados");save.type="submit";form.append(error,save);
 form.onsubmit=async e=>{e.preventDefault();if(save.disabled)return;save.disabled=true;try{
  if(store!==currentStoreId)throw Error("privacy_review_changed");
  const res=await fetch("/api/customer-delete?customer_id="+customerId,{method:"DELETE",headers:{"X-OpenFood":"1","X-Store-ID":String(store),"X-Privacy-Review":c.version}});
  if(!res.ok)throw Error((await res.json()).error||"anonymization_failed");
  await privacySync(true);dialog.close();$("lgpdDeleteConfirm").checked=false;await loadCustomers();await refresh();msg("Identificação e textos vinculados removidos. Valores financeiros preservados; revise os backups e arquivos anteriores.");
 }catch(err){error.textContent=menuError(err);error.classList.remove("hidden");save.disabled=false}};
 if(!dialog.open)dialog.showModal();confirm.focus();
}


// No personal identifiers in cross-tab messages or localStorage. Revisions are
// checked before each API mutation, on focus/reconnect and every three seconds.
const privacyFetch=window.fetch.bind(window);
let privacyReloading=false,privacySyncPromise=null;
const privacyShield=document.createElement("dialog");privacyShield.id="privacyShield";privacyShield.hidden=true;privacyShield.setAttribute("aria-label","Verificação de privacidade");privacyShield.textContent="Verificando privacidade. Reconecte para continuar.";privacyShield.addEventListener("cancel",e=>e.preventDefault());document.body.append(privacyShield);
function privacySetShield(active){
 if(active){document.querySelectorAll("dialog[open]").forEach(d=>{if(d!==privacyShield)d.close()});privacyShield.hidden=false;if(!privacyShield.open)privacyShield.showModal()}
 else {if(privacyShield.open)privacyShield.close();privacyShield.hidden=true}
}
function privacyClearDrafts(){
 for(const key of Object.keys(sessionStorage))if(key.startsWith("openfood:pos:"))sessionStorage.removeItem(key);
}
async function privacySync(accept=false){
 if(!currentUser||!currentStoreId||privacyReloading)return;
 const store=currentStoreId,user=currentUser.id,key=`openfood:privacy:${user}:${store}`;
 let res;try{res=await privacyFetch("/api/privacy-state",{headers:{"X-Store-ID":String(store)},cache:"no-store"})}catch(e){privacySetShield(true);throw e}
 if(!res.ok){privacySetShield(true);if(res.status===401){privacyClearDrafts();location.reload();return}throw Error("Não foi possível verificar a privacidade. Reconecte antes de continuar.")}
 const {revision}=await res.json();if(store!==currentStoreId||user!==currentUser?.id)return;
 privacySetShield(false);
 const old=sessionStorage.getItem(key);
 if(old&&old!==revision){
  privacyClearDrafts();sessionStorage.setItem(key,revision);
  // Reload clears in-memory carts, customer lists, free-text forms and dialogs.
  // Never resubmit an ambiguous payment attempt after clearing a draft.
  sessionStorage.setItem("openfood:privacy-notice","1");privacyReloading=true;
  document.documentElement.style.visibility="hidden";location.reload();
  if(!accept)throw Error("Os dados de privacidade mudaram. Reabrindo a operação.");
 }else {if(!old)privacyClearDrafts();sessionStorage.setItem(key,revision)}
}
window.fetch=async function(input,options){
 const path=typeof input==="string"?input:"";
 if(path.startsWith("/api/")&&currentUser&&currentStoreId&&!path.startsWith("/api/privacy-state")){
  if(!privacySyncPromise)privacySyncPromise=privacySync().finally(()=>privacySyncPromise=null);
  await privacySyncPromise;
  if(privacyReloading)throw Error("Atualizando privacidade");
 }
 return privacyFetch(input,options);
};
function privacyCheck(){retentionButton.classList.toggle("hidden",!currentUser||!canManageUsers());privacySync().catch(()=>{});}
setInterval(privacyCheck,3000);
window.addEventListener("focus",privacyCheck);
window.addEventListener("online",privacyCheck);
window.addEventListener("offline",()=>{if(currentUser)privacySetShield(true)});
document.addEventListener("visibilitychange",()=>{if(!document.hidden)privacyCheck()});
// Logout clears this tab's drafts even when the next operator uses the same PC.
const privacyLogout=$("logout").onclick;
$("logout").onclick=()=>{privacyClearDrafts();return privacyLogout()};
if(sessionStorage.getItem("openfood:privacy-notice")){
 sessionStorage.removeItem("openfood:privacy-notice");
 setTimeout(()=>msg("Privacidade atualizada: rascunhos e carrinhos em espera foram descartados. Confira o histórico de pedidos antes de repetir uma venda ou pagamento.","error"),1200);
}

Object.assign(menuErrors,{privacy_policy_confirmation_required:"Informe o prazo e confirme a ativação da limpeza.",privacy_policy_changed:"A política mudou. Reabra para conferir a versão atual.",privacy_policy_failed:"Não foi possível consultar ou salvar a política."});
const retentionButton=menuNode("button","btn-secondary","Política de retenção");retentionButton.id="privacyPolicyButton";
$("lgpdDeleteBtn").parentElement.after(retentionButton);
retentionButton.onclick=()=>openPrivacyPolicy().catch(e=>msg(menuError(e),"error"));
async function openPrivacyPolicy(){
 const store=currentStoreId,p=await api("privacy-policy");
 if(store!==currentStoreId)return;
 let d=$("privacyPolicyDialog");if(!d){d=menuNode("dialog","dialog menu-editor-dialog");d.id="privacyPolicyDialog";d.setAttribute("aria-labelledby","privacyPolicyTitle");document.body.append(d)}
 d.replaceChildren();const header=menuNode("div","dialog-header"),title=menuNode("h2","","Retenção de clientes");title.id="privacyPolicyTitle";
 const close=menuNode("button","icon-button","✕");close.type="button";close.setAttribute("aria-label","Fechar política de retenção");close.onclick=()=>d.close();header.append(title,close);d.append(header);
 const body=menuNode("div","dialog-body");d.append(body);
 body.append(menuNode("p","menu-hint","Defina o prazo da sua loja. A rotina remove identificação e textos vinculados; mantém valores, IDs e datas dos registros financeiros. Nenhum prazo legal é presumido."));
 body.append(menuNode("p","notice","Inatividade considera cadastro, consentimentos e pedidos. Pedidos em andamento, saldos pendentes e comandas abertas impedem a limpeza. A rotina verifica a cada minuto, em lotes de até 50 clientes. Backups e arquivos anteriores continuam exigindo guarda e descarte próprios."));
 const form=menuNode("form","menu-admin-form");form.id="privacyPolicyForm";body.append(form);
 const label=menuNode("label","","Dias sem atividade"),days=document.createElement("input");days.id="privacyRetentionDays";days.type="number";days.min=1;days.max=36500;days.step=1;days.value=p.inactive_days||"";label.append(days);form.append(label);
 const enabled=menuCheck(form,"Ativar limpeza automática nesta loja",p.enabled,"retention_enabled");
 const confirm=menuCheck(form,"Confirmo o prazo e autorizo a limpeza dos registros elegíveis",false,"retention_confirm");
 const preview=menuNode("p","menu-hint","Simule o prazo antes de ativar."),simulate=menuNode("button","btn-secondary","Simular impacto");simulate.type="button";simulate.id="privacyRetentionPreview";
 simulate.onclick=async()=>{try{const n=Number(days.value);if(!Number.isInteger(n)||n<1||n>36500)throw Error("Informe um prazo entre 1 e 36500 dias.");const result=await api("privacy-policy?days="+n);preview.textContent=result.candidates+" clientes elegíveis agora. A rotina revalida os registros antes da remoção."}catch(e){preview.textContent=menuError(e)}};
 const error=menuNode("p","pos-inline-error hidden");error.setAttribute("role","alert");const save=menuNode("button","","Salvar política");save.type="submit";save.setAttribute("form","privacyPolicyForm");form.append(simulate,preview,error);const footer=menuNode("div","privacy-policy-footer");footer.append(save);d.append(footer);confirm.onchange=()=>error.classList.add("hidden");
 form.onsubmit=async e=>{e.preventDefault();save.disabled=true;try{if(store!==currentStoreId)throw Error("privacy_policy_changed");await api("privacy-policy",{version:p.version,enabled:enabled.checked,inactive_days:Number(days.value)||0,confirm:confirm.checked});d.close();msg("Política salva. A limpeza automática segue o prazo e a ativação definidos nesta loja.")}catch(e){error.textContent=menuError(e);error.classList.remove("hidden");save.disabled=false}};
 d.showModal();days.focus();
}
