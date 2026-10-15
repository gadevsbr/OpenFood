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
 root.append(menuNode("p","notice","Rótulos de comandas vinculadas serão substituídos pelo número da comanda, inclusive quando compartilhadas com outros clientes. Backups e exportações anteriores não são alterados. Campos sem vínculo ao cliente exigem revisão manual. Não há prazo de retenção automático nem garantia de anonimato."));
 const form=menuNode("form","menu-admin-form");root.append(form);form.id="privacyRemovalForm";
 const confirm=menuCheck(form,"Confirmo a remoção de identificação e textos livres dos registros listados",false,"privacy_confirm");confirm.required=true;
 const error=menuNode("p","pos-inline-error hidden");error.setAttribute("role","alert");const save=menuNode("button","btn-cancel","Remover identificação e textos vinculados");save.type="submit";form.append(error,save);
 form.onsubmit=async e=>{e.preventDefault();if(save.disabled)return;save.disabled=true;try{
  if(store!==currentStoreId)throw Error("privacy_review_changed");
  const res=await fetch("/api/customer-delete?customer_id="+customerId,{method:"DELETE",headers:{"X-OpenFood":"1","X-Store-ID":String(store),"X-Privacy-Review":c.version}});
  if(!res.ok)throw Error((await res.json()).error||"anonymization_failed");
  dialog.close();$("lgpdDeleteConfirm").checked=false;await loadCustomers();await refresh();msg("Identificação e textos vinculados removidos. Valores financeiros preservados; revise os backups e arquivos anteriores.");
 }catch(err){error.textContent=menuError(err);error.classList.remove("hidden");save.disabled=false}};
 if(!dialog.open)dialog.showModal();confirm.focus();
}
