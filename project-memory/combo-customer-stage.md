# Combos e identificação de clientes — etapa 0.11
Skills: Backend Architect e Privacy Engineer. Implementação em validação;
sem declaração de conclusão das Fases 3 e 4.

- Combo é uma linha comercial com product_id nulo; componentes, quantidades,
  nomes/preços/opções/observações ficam no snapshot. Migração v8 separada preserva
  fingerprints v1–v7. APIs de configuração e vendas compartilham lock por loja.
- Preço: fixo + ajustes; soma; maior componente com quantidade. Estoques produto,
  variante e adicional são transacionais; cancelamento recompõe pelo snapshot.
- API valida composição, limites, loja, configuração e preço; UI cobre mobile/escuro.
- Cliente opcional por venda/rodada; vínculo validado na mesma loja, histórico no ADM.
  Busca de atendimento reduzida a nome/telefone, limitada a 100 resultados por consulta.
- Consentimento com texto obrigatório para concessão nova, finalidade/versão imutável,
  origem/evidência/data; revogação preservada e auditada. Preferência antiga de marketing
  não representa autorização formal. Financeiro somente consulta, sem mutação.
- Exportação repeatable-read inclui itens/combos/recebimentos e aborta em erro.
  Remoção auditada limpa identificação/contatos/observações/evidência; conserva
  histórico financeiro, IDs e datas. Não é certificação legal/anonimato garantido.
- Remoção somente manual; retenção automática e remoção após restaurar backups antigos
  pendentes. Backups/exportações anteriores podem conter dados. Não inventar prazo fiscal.
- Limites anteriores de combos indisponíveis e exportação que omitia erros: superseded
  pela implementação; conclusão depende de evidências finais.

Intermediário: Go completo PostgreSQL PASS 267.257s (antecede ajustes finais de
consentimento/UI); ComboCustomerJourney ampliado PASS 19.77s. Chromium combos/clientes
PASS, zero erros JS, capturas desktop/mobile/escuro inspecionadas. Repetição final,
regressões e CI/distribuição pendentes. Dados de teste isolados, sem dados de trabalho.

Gate local final: PostgreSQL completo PASS 314.815s, upgrades/restauração v1–v7;
DDL v8 normalizado LF para fingerprint Windows/Linux idêntico. Repetição focal
com fingerprint definitivo e backup/restauração PASS 56.11s (antes: 30.03s).
Chromium fresco combo-20261011-b: núcleo/setup/backup, combos/clientes e regressão
cardápio/mesas PASS; zero pageerrors. ADM PASS após selecionar fixture por nome
com email válido; falhas anteriores da automação superseded. Carrinho conserva
cliente/composição após reload; identificação opcional recolhida no mobile,
observações por componente visíveis e capturas finais inspecionadas. Financeiro
não altera cliente/consentimento; ambas as rotas de remoção compartilham transação.
Vet/sintaxe/diff PASS. Não certifica escopo completo de privacidade/anonimato.
CI/publicação ainda pendentes; provas em docs/evidence/combo-stage.
