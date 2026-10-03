# Spec Delta

## Purpose

Ofrece una página web simple para la demo del MVP: la persona se identifica con un nombre ficticio, ve el mercado y su cuenta, compra, vende y carga dinero, sin escribir comandos. La página solo consume la API existente de market, wallet y orders.

## ADDED Requirements

### Requirement: Identificación por nombre ficticio
La página MUST pedir un nombre ficticio y enviarlo como `X-User-ID` en cada llamada de wallet y orders. El nombre MUST conservarse al recargar la página y MUST poder cambiarse sin recargarla.

#### Scenario: Primera visita
- **WHEN** la persona abre la página sin haber elegido un nombre
- **THEN** la página le pide un nombre y no consulta `/wallet` ni envía órdenes hasta que lo haya elegido

#### Scenario: Nombre elegido
- **WHEN** la persona escribe `luna_vib` como nombre
- **THEN** cada llamada a `/wallet`, `/wallet/movements`, `/wallet/deposits` y `/orders` lleva el header `X-User-ID: luna_vib`

#### Scenario: Recarga de la página
- **WHEN** la persona que eligió `luna_vib` recarga la página
- **THEN** la página sigue con `luna_vib` y no vuelve a pedir el nombre

#### Scenario: Cambio de nombre
- **WHEN** la persona cambia su nombre a `beto`
- **THEN** la cuenta que se muestra pasa a ser la de `beto`, sin recargar la página

#### Scenario: Nombre vacío
- **WHEN** la persona borra el nombre y lo deja vacío
- **THEN** la página conserva el último nombre válido

### Requirement: Mercado a la vista
La página MUST mostrar la profundidad del book `BRL-VIB` de `GET /market/orderbook/BRL-VIB`: en una sola lista: las ventas (`asks`) arriba y las compras (`bids`) abajo, con el spread en medio. Cada nivel muestra su precio en BRL, su volumen en VIB y el volumen acumulado desde el mejor precio, y el mejor precio de cada lado queda junto al spread: la compra más alta y la venta más baja. Cada lado MUST tener siempre el mismo número de filas, con las que faltan vacías, y MUST actualizarse sin recrearse. MUST mostrarse sin que la persona haya elegido un nombre.

#### Scenario: Book con niveles
- **WHEN** el mercado responde `bids = [{price:"100.00", volume:"30"}, {price:"99.00", volume:"15"}]` y `asks = [{price:"101.00", volume:"5"}]`
- **THEN** la página muestra las compras de R$ 100.00 con 30 VIB y de R$ 99.00 con 15 VIB, y la venta de R$ 101.00 con 5 VIB

#### Scenario: Volumen acumulado
- **WHEN** el mercado responde `bids = [{price:"100.00", volume:"30"}, {price:"99.00", volume:"15"}]`
- **THEN** la página muestra 30 VIB acumulados en R$ 100.00 y 45 VIB acumulados en R$ 99.00, con la barra del nivel proporcional al acumulado

#### Scenario: Spread entre ambos lados
- **WHEN** la mejor compra es R$ 99.81 y la mejor venta es R$ 100.02
- **THEN** la página muestra un spread de R$ 0.21 entre las ventas y las compras

#### Scenario: Un lado vacío
- **WHEN** el mercado responde `asks = []`
- **THEN** la página muestra el lado de ventas con todas sus filas vacías, sigue mostrando las compras y la lista no cambia de altura

#### Scenario: Sin nombre elegido
- **WHEN** la persona todavía no eligió un nombre
- **THEN** la página muestra igualmente el mercado

### Requirement: Refresco periódico
Mientras la página está abierta, MUST volver a consultar la profundidad del mercado, el wallet, los movimientos y las órdenes cada 2 segundos y repintar todo con las respuestas. MUST hacer la misma consulta justo después de una escritura exitosa, sin esperar al siguiente ciclo.

#### Scenario: Cambio en el mercado
- **WHEN** el book cambia y pasan 2 segundos
- **THEN** la página muestra la nueva profundidad sin intervención de la persona

#### Scenario: Después de operar
- **WHEN** la persona envía una orden o carga dinero y la API responde `201`
- **THEN** la página vuelve a consultar el mercado, el wallet, los movimientos y las órdenes de inmediato

#### Scenario: Falla una consulta
- **WHEN** una consulta del ciclo falla o no responde
- **THEN** la página conserva lo último que mostró y lo vuelve a intentar en el siguiente ciclo

### Requirement: Operar con los 4 casos de orden
La página MUST permitir comprar y vender, y en cada caso elegir entre orden a Límite o a Mercado, con Comprar y Límite preseleccionados. MUST mostrar solo los campos del caso elegido y enviar `POST /orders` con el book `BRL-VIB`, el `side` elegido y los montos como strings decimales.

#### Scenario: Compra a límite
- **WHEN** la persona elige Comprar y Límite, escribe precio `90.00` y cantidad `10` y confirma
- **THEN** envía `POST /orders` con `side = "BUY"`, `limit = "90.00"` y `quantity = "10"`, sin `amount`

#### Scenario: Venta a límite
- **WHEN** la persona elige Vender y Límite, escribe precio `95.00` y cantidad `3` y confirma
- **THEN** envía `POST /orders` con `side = "SELL"`, `limit = "95.00"` y `quantity = "3"`, sin `amount`

#### Scenario: Compra a mercado
- **WHEN** la persona elige Comprar y Mercado, escribe monto `500.00` y confirma
- **THEN** envía `POST /orders` con `side = "BUY"` y `amount = "500.00"`, sin `limit` ni `quantity`

#### Scenario: Venta a mercado
- **WHEN** la persona elige Vender y Mercado, escribe cantidad `5` y confirma
- **THEN** envía `POST /orders` con `side = "SELL"` y `quantity = "5"`, sin `limit` ni `amount`

#### Scenario: Campos según el caso
- **WHEN** la persona cambia de Límite a Mercado al comprar
- **THEN** la página oculta precio y cantidad y muestra el monto a gastar

#### Scenario: Orden enviada
- **WHEN** la API responde `201` a la orden
- **THEN** la página muestra que la orden fue enviada y cuánto dinero se reservará

### Requirement: Validación antes de enviar
La página MUST NOT enviar una orden o un depósito cuyo valor no sea un número mayor que 0 con los decimales de su moneda (2 para BRL, 0 para VIB), y MUST NOT enviar una orden que reserve más de lo disponible de la última lectura del wallet. En cada caso MUST decir el motivo. Un error de la API MUST mostrarse con su mensaje.

#### Scenario: Precio inválido
- **WHEN** la persona escribe precio `abc` o `90.123` y confirma
- **THEN** la página no envía la orden y muestra el motivo

#### Scenario: Cantidad con decimales
- **WHEN** la persona escribe cantidad `2.5` y confirma
- **THEN** la página no envía la orden y pide un número entero de VIB

#### Scenario: Saldo insuficiente
- **WHEN** la persona tiene R$ 100.00 disponibles y confirma una compra a límite de 10 VIB a R$ 90.00
- **THEN** la página no envía la orden y dice que el saldo no alcanza

#### Scenario: Error de la API
- **WHEN** la API responde `400` con `error.message = "invalid quantity"` o `503` con `service_unavailable`
- **THEN** la página muestra el mensaje del error y no da la orden por enviada

### Requirement: Mis órdenes
La página MUST listar las órdenes de la persona (de `GET /orders`, la más reciente primero) con su lado, su descripción, su estado, lo ejecutado y la hora. MUST ofrecer el filtro Activas | Todas, con Activas por defecto; activas son las órdenes `PENDING`, `OPEN` y `PARTIALLY_FILLED`.

#### Scenario: Solo las activas
- **WHEN** la persona tiene una orden `OPEN`, una `FILLED` y una `CANCELLED` y el filtro es Activas
- **THEN** la lista muestra solo la orden `OPEN`

#### Scenario: Ver todas
- **WHEN** la persona cambia el filtro a Todas
- **THEN** la lista muestra las tres órdenes, la más reciente primero

#### Scenario: Sin órdenes
- **WHEN** la persona no tiene órdenes, o todavía no eligió un nombre
- **THEN** la lista aparece vacía y dice que no hay órdenes

### Requirement: Lo ejecutado de cada orden
Cada orden MUST mostrar cuánto se ejecutó (`filledQuantity`) y a qué precio promedio (`avgPrice`). Si tiene `quantity`, MUST mostrarlo contra el total de la orden. Cuando la orden está `CANCELLED` o `REJECTED`, MUST mostrar la razón: la de `reason` cuando viene, y «cancelada por ti» cuando una cancelada no trae razón.

#### Scenario: Orden con ejecución parcial
- **WHEN** una orden `PARTIALLY_FILLED` de `quantity = "10"` tiene `filledQuantity = "4"` y `avgPrice = "90.50"`
- **THEN** la página muestra que se ejecutaron 4 de 10 VIB a un promedio de R$ 90.50

#### Scenario: Orden sin ejecución
- **WHEN** una orden `OPEN` tiene `filledQuantity = "0"` y `avgPrice = null`
- **THEN** la página muestra que todavía no se ejecutó nada

#### Scenario: Compra a mercado
- **WHEN** una compra a mercado tiene `quantity = null` y `filledQuantity = "3"`
- **THEN** la página muestra que se ejecutaron 3 VIB, sin un total

#### Scenario: Cancelada sin liquidez
- **WHEN** una orden `CANCELLED` trae `reason = "no_liquidity"`
- **THEN** la página muestra que se canceló por falta de liquidez en el mercado

#### Scenario: Cancelada por la persona
- **WHEN** una orden `CANCELLED` no trae `reason`
- **THEN** la página muestra que fue cancelada por la persona

#### Scenario: Orden rechazada
- **WHEN** una orden `REJECTED` trae `reason = "insufficient_funds"`
- **THEN** la página muestra que se rechazó por fondos insuficientes

### Requirement: Cancelar una orden
La página MUST mostrar un botón Cancelar en cada orden activa y MUST NOT mostrarlo en las que ya son finales. Al confirmarlo MUST enviar `POST /orders/{orderId}/close`, y la orden MUST mostrar su resultado en el siguiente refresco, porque la cancelación se aplica de forma asíncrona.

#### Scenario: Cancelar una orden abierta
- **WHEN** la persona pulsa Cancelar en una orden `OPEN`
- **THEN** envía `POST /orders/{orderId}/close` con su `X-User-ID`, dice que pidió la cancelación y vuelve a consultar de inmediato

#### Scenario: Cancelación aplicada
- **WHEN** la cancelación ya se aplicó y llega el siguiente refresco
- **THEN** la orden pasa a `CANCELLED`, el botón Cancelar desaparece y el filtro Activas ya no la muestra

#### Scenario: Orden final
- **WHEN** una orden está `FILLED`, `CANCELLED` o `REJECTED`
- **THEN** la página no muestra el botón Cancelar en esa orden

#### Scenario: Error al cancelar
- **WHEN** la API responde un error al cancelar
- **THEN** la página muestra el mensaje del error

### Requirement: Cuenta y movimientos
La página MUST mostrar los saldos de BRL y VIB de la persona (total y reservado, de `GET /wallet`) y sus 5 movimientos más recientes (de `GET /wallet/movements`), el más nuevo primero, con tipo, moneda, monto con signo y hora. MUST reconocer los tipos `DEPOSIT`, `WITHDRAWAL`, `RESERVE`, `RELEASE`, `TRADE_PAID` y `TRADE_RECEIVED`.

#### Scenario: Saldos
- **WHEN** el wallet responde BRL con `available = "100.00"` y `reserved = "900.00"`, y VIB con `available = "5"` y `reserved = "0"`
- **THEN** la página muestra R$ 1,000.00 en total con R$ 900.00 reservados, y 5 VIB con 0 reservados

#### Scenario: Movimientos recientes
- **WHEN** la persona tiene más de 5 movimientos
- **THEN** la página muestra los 5 más nuevos, en orden del más nuevo al más viejo

#### Scenario: Reserva al operar
- **WHEN** la persona envía una compra a límite de 10 VIB a R$ 90.00 y el engine la acepta
- **THEN** en el siguiente refresco aparece un movimiento de reserva por R$ 900.00 y el reservado de BRL sube en esa cantidad

#### Scenario: Cuenta nueva
- **WHEN** la persona elige un nombre que nunca operó
- **THEN** la página muestra saldos en cero y la lista de movimientos vacía

### Requirement: Cargar dinero
La página MUST permitir cargar BRL o VIB: la persona elige la moneda, escribe el monto y confirma, y la página envía `POST /wallet/deposits` con `currency` y `amount` como string decimal.

#### Scenario: Cargar BRL
- **WHEN** la persona elige BRL, escribe `150.00` y confirma
- **THEN** envía `POST /wallet/deposits` con `currency = "BRL"` y `amount = "150.00"`

#### Scenario: Cargar VIB
- **WHEN** la persona elige VIB, escribe `10` y confirma
- **THEN** envía `POST /wallet/deposits` con `currency = "VIB"` y `amount = "10"`

#### Scenario: Depósito reflejado
- **WHEN** la API responde `201` al depósito
- **THEN** el disponible de esa moneda sube por el monto y aparece un movimiento de depósito, sin recargar la página

#### Scenario: Monto inválido
- **WHEN** la persona escribe `0` o `10.555` en BRL y confirma
- **THEN** la página no envía el depósito y muestra el motivo

### Requirement: Servida junto a la API
La página MUST servirse desde `/` en el mismo host y puerto que la API, y MUST llamar a la API con rutas relativas, de modo que el navegador no necesite CORS. Las rutas `/orders`, `/wallet` y `/market` MUST seguir siendo las de la API.

#### Scenario: Abrir la página
- **WHEN** se hace `GET /` en el host de la API
- **THEN** la respuesta es la página HTML de la demo

#### Scenario: La API no cambia
- **WHEN** se hace `GET /market/orderbook/BRL-VIB` en el mismo host
- **THEN** responde el mercado, no la página
