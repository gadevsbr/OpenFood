# Catálogo e pedidos — 0.5.0-alpha.1

Skills aplicadas: Frontend Developer e Code Reviewer (Agency Agents local).

Implementados: edição de produtos com estoque esperado e auditoria transacional; carrinho agrupado, total estimado e confirmação; detalhes com snapshots; disponibilidade/horários e quantidades validados no servidor. Horário America/Sao_Paulo; pedidos por peso/variantes/adicionais seguem pendentes. Sem migração nova.

Evidência: suíte real PostgreSQL inicial PASS 84.959s. Chromium real: regressão e jornada nova PASS, zero pageerrors; carrinho de dois itens total 9150 centavos, cancelamento restaura ambos os estoques e editor aberto rejeita venda concorrente. Capturas carrinho/detalhe mobile inspecionadas. Repetição após auditoria PASS 73.555s (suíte completa); expansão final dos testes PASS 26.510s (TestPostgresJourney). Vet, sintaxe JS e diff-check PASS. govulncheck zero vulnerabilidades alcançáveis, uma em módulo requerido sem chamadas. Pipeline [38105590515](https://github.com/gadevsbr/OpenFood/actions/runs/38105590515): Linux, Windows (PostgreSQL/migrações e ciclo instalado) e publicação PASS. [Release experimental](https://github.com/gadevsbr/OpenFood/releases/tag/v0.5.0-alpha.1) com seis assets; quatro artefatos baixados e SHA-256 igual aos manifests. Ambiente limpo/assinatura/reboot permanecem abertos.

Pagamentos manuais e fechamento de caixa são a próxima etapa, não implementados aqui.
