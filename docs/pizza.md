# Pizzas com múltiplos sabores

## Cadastro

ADM → Catálogo → produto → **Tamanhos, adicionais e foto** → **Configurar sabores de pizza**.

Use um produto vendido por **unidade**. Ative sabores, defina o máximo (1 a 4), cadastre os sabores e seus preços e salve. Cada preço representa uma pizza inteira no tamanho base. O ajuste do tamanho, configurado em variantes, é comum a todos os sabores; adicionais são somados depois. Até 30 sabores podem ser cadastrados; desmarque Disponível para retirar um sabor do atendimento.

As configurações têm versão: se outro operador alterar o produto, reabra a tela antes de salvar. O mesmo vale para carrinhos montados com configurações antigas.

## Atendimento e preço

No PDV, escolha tamanho, sabores e partes. **1 + 1** divide meio a meio; **2 + 1** divide em dois terços e um terço. Até oito partes inteiras no total. Não repita o mesmo sabor em duas escolhas.

- **Sabor mais caro:** usa o maior preço inteiro entre os sabores selecionados, independentemente da proporção.
- **Média proporcional às partes:** soma preço × partes e divide pelo total de partes. Arredonda ao centavo mais próximo; meio centavo arredonda para cima.

Exemplo: Calabresa R$ 30,01 e Queijo R$ 40,02, meio a meio, resultam em R$ 35,02 pela média. Com ajuste de tamanho de R$ 5,00 e borda de R$ 3,00, a pizza custa R$ 43,02. Em 2/3 Calabresa + 1/3 Queijo, o total com os mesmos extras é R$ 41,35. Pela regra do maior preço, fica R$ 48,02 com esses extras.

O servidor recalcula tudo em centavos. O total visto no carrinho é uma prévia; alteração de preço/configuração rejeita a confirmação e exige revisão. O estoque é transacional, sem consumir novamente em retry da mesma operação.

## Mesas, combos e registros

O mesmo configurador funciona no PDV e nas rodadas de mesa/comanda. Pizzas podem compor combos; a regra de preço do combo continua aplicada sobre os componentes configurados. Em combo de preço fixo, diferenças do preço do sabor em relação ao preço base do produto também entram como ajustes.

Sabores, proporções, tamanho, adicionais e observações ficam registrados na venda. Alterações posteriores do cardápio não mudam os pedidos existentes. Cozinha e comprovantes recebem os nomes/proporções; o snapshot também preserva política e preços.

## Estoque e limites

Uma pizza baixa uma unidade do produto e do tamanho, independentemente dos sabores ou partes. Adicionais baixam uma unidade por pizza quando escolhidos. O cancelamento recompõe esses estoques. Não há estoque de ingredientes por grama ou por fração de sabor nesta etapa.

Os limites de sabores são por produto, não por tamanho. O ajuste de tamanho é comum aos sabores; não há tabela independente de preço por par tamanho/sabor. Venda por peso, adicionais compartilhados e critérios completos de privacidade permanecem no roadmap. A versão continua experimental.
