Object.assign(menuErrors,{schedule_invalid:"Confira os dias e horários. Faixas do mesmo dia não podem se sobrepor, inclusive após meia-noite.",schedule_not_found:"Loja ou categoria não disponível.",schedule_failed:"Não foi possível salvar os horários. Reabra o cadastro e tente novamente."});
const saleDays=["Domingo","Segunda-feira","Terça-feira","Quarta-feira","Quinta-feira","Sexta-feira","Sábado"];
const saleTime=n=>`${String(Math.floor(n/60)%24).padStart(2,"0")}:${String(n%60).padStart(2,"0")}`;
async function scheduleEditor(category=0){
 const store=currentStoreId,c=await api("sale-schedule?category_id="+category);
 if(store!==currentStoreId)return;
 let dialog=$("scheduleEditorDialog");
 if(!dialog){dialog=menuNode("dialog","dialog menu-editor-dialog");dialog.id="scheduleEditorDialog";dialog.setAttribute("aria-labelledby","scheduleEditorTitle");dialog.innerHTML='<div class="dialog-header"><h2 id="scheduleEditorTitle"></h2><button type="button" class="icon-button" aria-label="Fechar horários">✕</button></div><div class="dialog-body"></div>';dialog.querySelector("button").onclick=()=>dialog.close();document.body.append(dialog)}
 $("scheduleEditorTitle").textContent=c.name+" · horários de venda";
 const root=dialog.querySelector(".dialog-body");root.replaceChildren();
 root.append(menuNode("p","menu-hint",category?"A categoria também respeita os horários da loja, das categorias superiores e de cada produto. Pedidos registrados continuam disponíveis para preparo e recebimento.":"Defina quando a loja aceita novos itens. Categorias e produtos podem restringir ainda mais o horário. Pedidos registrados continuam disponíveis para preparo e recebimento."));
 const form=menuNode("form","menu-admin-form");form.id="saleScheduleForm";root.append(form);
 const zoneLabel=menuNode("label","","Fuso horário da loja"),zone=document.createElement("select");zone.name="schedule_timezone";
 for(const z of [...new Set([c.timezone,"America/Sao_Paulo","America/Manaus","America/Rio_Branco","America/Noronha","America/Cuiaba","America/Campo_Grande","America/Recife","America/Bahia","America/Fortaleza","America/Belem","UTC"])])zone.append(new Option(z.replaceAll("_"," "),z));zone.value=c.timezone;zone.disabled=!!category;zoneLabel.append(zone);form.append(zoneLabel);
 const enabled=menuCheck(form,"Limitar vendas aos horários abaixo",c.schedule.enabled,"schedule_enabled");
 form.append(menuNode("p","menu-hint","Sem limite: disponível em qualquer dia, respeitando as demais regras. Com limite e sem faixas: fechado a semana inteira. Fechar às 00:00 significa meia-noite; 00:00 até 00:00 cobre o dia inteiro. A abertura inclui o minuto inicial, e o fechamento encerra a faixa."));
 const windows=menuNode("div","sale-schedule-windows"),entries=[];form.append(windows);
 function add(w={day:1,start:540,end:1080}){
  const row=menuNode("div","menu-admin-row");windows.append(row);
  const label=menuNode("label","","Dia"),day=document.createElement("select");day.name="schedule_day";saleDays.forEach((name,i)=>day.append(new Option(name,i)));day.value=w.day;label.append(day);row.append(label);
  const start=menuField(row,"Abre às",saleTime(w.start),"schedule_start","time"),end=menuField(row,"Fecha às",saleTime(w.end),"schedule_end","time");start.required=end.required=true;
  const hint=menuNode("p","menu-hint"),entry={day,start,end};entries.push(entry);
  const update=()=>{const a=start.value,b=end.value;hint.textContent=a&&b&&b!=="00:00"&&b<a?"Fecha no dia seguinte.":a==="00:00"&&b==="00:00"?"Dia inteiro, até meia-noite.":"Horário do dia selecionado."};start.onchange=end.onchange=update;update();row.append(hint,posButton("Remover faixa",()=>{entries.splice(entries.indexOf(entry),1);row.remove()}));
 }
 c.schedule.windows.forEach(add);form.append(posButton("Adicionar faixa",()=>{if(entries.length<42)add()}));
 const confirmation=menuCheck(form,"Aplicar aos produtos desta loja; atualizar itens em carrinhos abertos",false,"schedule_confirm");confirmation.required=true;
 const error=menuNode("p","pos-inline-error hidden");error.setAttribute("role","alert");const save=menuNode("button","","Salvar horários");save.type="submit";form.append(error,save);
 const minute=s=>Number(s.slice(0,2))*60+Number(s.slice(3,5));
 form.onsubmit=async e=>{e.preventDefault();if(save.disabled)return;save.disabled=true;try{
  if(store!==currentStoreId)throw Error("configuration_changed");
  await api("sale-schedule?category_id="+category,{...c,timezone:zone.value,schedule:{enabled:enabled.checked,windows:entries.map(x=>({day:Number(x.day.value),start:minute(x.start.value),end:minute(x.end.value)||1440}))}});
  dialog.close();await refresh();msg("Horários salvos. Confira e atualize os itens em carrinhos abertos.");
 }catch(err){error.textContent=menuError(err);error.classList.remove("hidden");save.disabled=false}};
 if(!dialog.open)dialog.showModal();enabled.focus();
}
$("subpanelCategorias").firstElementChild.append(posButton("Horários da loja",()=>scheduleEditor().catch(e=>msg(menuError(e),true))));
