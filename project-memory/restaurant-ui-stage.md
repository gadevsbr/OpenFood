# Direção restaurante — 2026-10-11

Skills: UI Designer, UX Architect, UI Finish-Gate Reviewer.
Usuário: atendente encontra produtos, monta pedido e recebe; gestor consulta resultados.
Primeira leitura: cardápio e pedido; ação principal: adicionar produto.

- Pedido explícito: referência https://www.ifood.com.br/inicio. Site direto 403/blank;
  captura pública conferida em https://community.e-com.plus/t/transformar-banner-grid-em-slider-na-versao-mobile/4155.
- Padrões adaptados: cabeçalho branco, busca cinza, categorias visuais, seleção/ação
  vermelha, nomes/descrições/preços hierarquizados e navegação leve. Sem banners
  promocionais de marketplace dentro do terminal de caixa.
- Verde floresta/lima e produtos com iniciais: **superseded** por neutros/vermelho e
  cardápio descritivo, conforme pedido do usuário. Fluxos e status semânticos mantidos.
- Quatro fotografias Unsplash ilustram categorias reconhecidas; categorias próprias
  usam alternativa neutra. Produtos mostram dados reais, sem fotos atribuídas.
  Imagens locais embutidas em Go; licença vendor/FOOD-IMAGES-LICENSE.txt.
- Mobile: lista única, categorias/atalhos horizontais e pedido acessível no rodapé.
  Temas claro/escuro/sistema, foco e movimento reduzido preservados.
- Proibidos: gradientes/glass, métricas fictícias, fotos ilustrativas como produtos
  reais, renomear categorias cadastradas e dependências CDN durante o uso.
- Gate visual: **PASS no escopo testado**. Capturas 1440/768/390/320 e escuro
  inspecionadas; regressões setup/admin/caixa/PDV/mesas PG/Chromium PASS, zero pageerrors.
  Sobreposição inicial do primeiro produto em 320 corrigida e retestada.
  Contrastes claros >= 5.11 nos tokens auditados; zero requests externos/assets falhos.
  Não certifica WCAG completa/leitor de tela. Sem mudança de schema.
- Evidências: docs/evidence/restaurant-ui; PostgreSQL completo PASS 121.825s, vet,
  sintaxe/diff e rebuild Motion PASS. Produto permanece experimental.

Distribuição: [pipeline 38187157507](https://github.com/gadevsbr/OpenFood/actions/runs/38187157507) Windows/Linux/publish PASS. Seis assets publicados na [release experimental v0.9.0-alpha.1](https://github.com/gadevsbr/OpenFood/releases/tag/v0.9.0-alpha.1); instalador/ZIP Windows, TAR Linux e ZIP de evidências baixados, quatro SHA-256 iguais aos manifests. Pendência de distribuição: **superseded** por esta evidência.
