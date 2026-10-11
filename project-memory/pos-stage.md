# PDV e ADM — contrato 2026-10-11

Skills: UI Designer, UX Architect, UI Finish-Gate Reviewer. Escopo em execução; sem declaração de pronto.

Após login: duas metades PDV | ADM. PDV é posto de venda com catálogo/busca/código/categorias à esquerda e carrinho à direita; ADM mantém gestão e resumo com dados reais. Botão superior troca ambientes; troca não promove permissões. Caixa fechado bloqueia venda e exige abertura. Atendente passa a poder abrir caixa e fechar o que abriu; gerentes administram qualquer caixa da loja. Decisão 0.6 de abertura exclusivamente gerencial: superseded por pedido explícito do usuário.

PDV: quantidades, remoção/limpeza, pagamentos divididos, dinheiro entregue/troco, comprovante não fiscal, venda balcão ou envio para preparo, descontos gerenciais com motivo, recebimentos pendentes, devoluções gerenciais, sangria/suprimento auditados, abertura/fechamento. Checkout atômico usa preços/estoque do servidor, caixa aberto, valor esperado e chave estável; erro não deixa venda parcial. Peso/variantes/adicionais, NF-e/NFC-e/TEF e confirmação bancária exigem etapas específicas e não serão apresentados como operacionais.

Visual: verde floresta/lima, superfície mineral, tipografia local. Escolha dividida sem cartões pequenos; PDV sem sidebar administrativa; no mobile carrinho vira área com acesso direto. Animações curtas não bloqueiam ações e respeitam reduced-motion.

Referências oficiais: Square item grid (https://squareup.com/help/us/en/article/8334-set-up-item-grid), Shopify smart grid (https://help.shopify.com/en/manual/sell-in-person/getting-started/smart-grid), processamento de venda (https://help.shopify.com/en/manual/sell-in-person/shopify-pos/process-sales-transaction).

Bibliotecas pesquisadas: Motion animate mini (https://motion.dev/docs/animate), Anime.js (https://animejs.com/documentation/getting-started/installation/). Motion 14.1.0 escolhida pelo suporte vanilla/WAAPI e bundle mini; Anime.js tem motor/timelines mais amplos que as transições necessárias. Bundle local com esbuild 0.28.2 e licença MIT; sem serviço pago ou recurso de Motion+.

Evidências exigidas: PostgreSQL real para checkout/caixa/concorrência/migrações; Chromium para escolha, troca, caixa fechado, venda/troco/desconto/comprovante, permissões, teclado, reduced-motion, temas e desktop/mobile. Distribuição experimental com checksums.

Preferência confirmada: restaurante, com categorias e produtos em destaque; preparo é o padrão do PDV. Modelo misto como prioridade visual: superseded pela resposta do usuário. Código/SKU permanece como busca auxiliar.

Validação parcial concluída: PostgreSQL real `go test ./...` PASS 107.124s, upgrades v1/v2/v3/v4 e restauração v3/v4. Chromium PDV PASS com categorias, pagamento/troco, desconto, rascunho, resposta perdida sem duplicar, sangria/histórico, atendente abrir/fechar próprio caixa e financeiro sem acesso PDV. Nova repetição integrada em base vazia em execução; publicação ainda pendente. `go vet` PASS; govulncheck zero alcançáveis e uma em módulo requerido sem chamada.

Repetição final integrada em base vazia pos-20261011-b: jornadas setup/UI, ADM, consentimento/offline/foco, caixa, caixa-final, PDV e vendas PASS; zero pageerrors. PostgreSQL final PASS 117.870s, incluindo quatro retries concorrentes POS e conflito de payload. Capturas PDV/ADM desktop/mobile/escuro revisadas; contraste hover ADM e posição do cabeçalho dashboard corrigidos. Gates locais atendidos no escopo, sem certificação de leitor de tela/periféricos/fiscal/Windows limpo. CI/publicação ainda pendentes.
