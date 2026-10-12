# Cardápio configurável — etapa 0.10

Skills: Backend Architect e UX Architect. Implementação autorizada em 2026-10-11;
gate local PASS no escopo testado; distribuição ainda aguarda CI/checksums.

- Variantes são ajustes sobre o preço base; produto com variantes ativas exige uma.
  Estoque é baixado no produto e na variante, por unidade vendida.
- Adicionais: uma seleção por opção, mínimo/máximo por grupo, obrigatório respeitado;
  estoque da opção baixa por unidade do item. Grupo é vinculado explicitamente ao produto.
- Linhas distinguem variante, adicionais e observação; escolhas idênticas são agrupadas.
  Servidor valida vínculo/loja, disponibilidade, limites, preço e versão da configuração.
- Snapshot registra composição/preço/nomes/observação no pedido; edição posterior não
  altera cozinha, conta ou comprovante. Cancelamento repõe os três estoques uma única vez.
- ADM: editor contextual por produto, estoque esperado e versão para impedir sobrescrita
  de venda concorrente. Grupos compartilhados ficam somente leitura no editor contextual;
  edição global conjunta fica pendente. Outros produtos não são alterados silenciosamente.
- Fotos JPEG/PNG locais: limite de bytes/pixels, decodificação e nova codificação JPEG;
  armazenamento no PostgreSQL, isolamento por loja e inclusão em backup/restauração.
- Migração v7 separada preserva fingerprints v3–v6 e pedidos anteriores.
- Checkout simples, pagamentos, descontos, comandas e estoque anterior permanecem.
  Limite anterior de variantes/adicionais indisponíveis: **superseded** pelo PASS local.
  Combos, sabores de pizza, peso, estoque por ingrediente e impressão física ficam fora.
- Aceite: hambúrguer/tamanho/adicionais/sem cebola em mesa, preço correto/preparo,
  conta e comprovante; manipulação de IDs/preços, limites, estoque, concorrência,
  retries, cancelamento, fotos inválidas, restauração e mobile/tema/foco.

Validação local: PostgreSQL completo PASS 262.972s (upgrades v1–v6/backup antigo).
TestMenuJourney final PASS 24.68s; regressões setup/ADM/caixa/PDV/mesas e cardápio
Chromium PASS, zero pageerrors. Captação/remoção de metadados da foto, acesso por
loja, recodificação JPEG e restauração verificadas no PostgreSQL. Capturas desktop,
ADM mobile/escuro, escolhas mobile/escuro, carrinho, recibo e cozinha inspecionadas.
Total/ação ficam fixos no diálogo de opções; salvar foto não descarta o formulário.
Falhas de bootstrap/migração, leitura nullable e compatibilidade de PATCH corrigidas.
Fixture e rota da automação corrigidas; HOLD local anterior superseded por PASS.
Vet/sintaxe/rebuild Motion PASS. Evidências: docs/evidence/menu-stage.
Não certifica todos os critérios da Fase 3, WCAG/leitor de tela, hardware ou fiscal.
