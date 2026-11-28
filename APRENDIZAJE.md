# APRENDIZAJE

Registro de sesiones de mentoría. Cada sesión se agrega al final; no se borra nada.

---

## Sesión 1 — 2026-11-25 — Arquitectura del gestor de turnos (diseño, sin código)

**Objetivo de la sesión:** diseñar la arquitectura de un sistema de turnos para laboratorio
clínico (`PROBLEMA.md`), con foco en resiliencia, colas y concurrencia, sin sobreingeniería y
sin decidir todavía tecnologías.

**Punto de partida:** el peor fallo imaginado era "la pantalla de la sala se desincroniza y
vuelve a llamar a todos". Sirvió como hilo conductor de toda la sesión.

---

### 1. Conceptos que aprendí

**Evento ≠ estado.** Un evento ("llamá a EP-004 al Box 1") ocurre una vez y no se re-emite.
El estado ("la sala está así") se puede pedir en cualquier momento y siempre es correcto.
La resiliencia de un cliente que se reconecta sale gratis si el cliente **solo sabe pedir
estado**, nunca reconstruir a partir de eventos perdidos.

**Idempotencia del lado del cliente.** El cliente no obedece comandos: compara el estado
recibido contra lo último que dibujó y actúa solo si cambió. Es lo mismo que hace una API
con una `idempotency-key`, pero del otro lado del cable.

**Una decisión, un dueño.** La pantalla renderiza. El servidor decide el orden y el estado.
El operador ejecuta acciones (llamar, rellamar, marcar ausente). Cuando dos componentes
pueden tomar la misma decisión, hay un bug esperando la hora pico.

**El candado es el `WHERE`.** No hace falta un `mutex` explícito: alcanza con una sentencia
que elige y marca en el mismo acto, condicionada al estado esperado
(`... WHERE estado = 'EN_ESPERA'`). Si afecta 1 fila, gané; si afecta 0, perdí la carrera y
reintento. No preguntes y después actúes: actuá condicionado a que el estado siga siendo el
que creías.

**Granularidad del bloqueo.** Un candado más grande de lo necesario mata el throughput; uno
más chico deja pasar corrupción. Acá la unidad correcta es **el servicio**, no el sistema
completo: dos tickets de circuitos distintos nunca compiten.

**Un ticket no se borra, cambia de estado.** Borrar es perder trazabilidad, métricas y al
paciente. Todo el ciclo de vida es una máquina de estados.

**Prioridad efectiva = f(categoría, tiempo de espera).** La prioridad no es un atributo del
paciente, es una función que se recalcula al momento de decidir. Así el envejecimiento
(aging) sale gratis y no hace falta ningún contador compartido entre boxes.

**Starvation.** El mismo problema que resuelven los sistemas operativos con *priority
scheduling + aging*. Transferir ese concepto a las colas de negocio es exactamente correcto.

**Una garantía se implementa como escalón, no como tendencia.** El tope de 45 minutos tiene
que ser un tier duro en el `ORDER BY`, no un *boost* en el score: un score suave puede seguir
perdiendo contra una cola larga de prioritarios.

**Un ticket puede tener varios relojes.** La espera total (desde el ingreso, para la regla de
justicia) y la espera por segmento (desde que entró a cada cola, para estimados y métricas)
son dos números distintos que responden dos preguntas distintas.

---

### 2. Decisiones tomadas y por qué

| Decisión | Alternativa descartada | Motivo |
|---|---|---|
| El TV es una proyección de solo lectura: pide el estado, lo dibuja, no decide nada | El TV escucha eventos y los aplica/reproduce | Si se corta y se pierde eventos, no sabe qué se perdió. Termina anunciando de más o de menos. |
| Snapshot con `llamado_actual` (con `id_llamado` y antigüedad), historial de últimos 4-5 llamados, estimados por servicio, `version` monotónica y estado de jornada | Mandar solo la lista del historial | El llamado actual quedaría inferido del "último de la lista", que no distingue llamado de atendido. La `version` evita retroceder ante respuestas fuera de orden. |
| El TV suena solo si `id_llamado` es nuevo **y** la antigüedad es menor a N segundos | Sonar siempre que el `id_llamado` cambie respecto de la memoria local | La memoria del cliente no es fuente de verdad: en arranque en frío está vacía y volvería a anunciar un llamado viejo (el fallo original). |
| Un solo conjunto de candidatos por servicio, ordenado por prioridad efectiva | Dos colas rígidas: prioritaria y general | Las dos colas producen starvation de los generales, y combinarlas con "2 a 1" exige un contador compartido entre 6 boxes: estado distribuido que hay que sincronizar y recuperar. |
| Aging como criterio de justicia | Proporción estricta 2:1 | El enunciado permite explícitamente cualquiera de las dos; aging es más simple y no necesita estado compartido. |
| Tope de 45 min como tier duro del orden | Tope como *boost* en el score | Debe ser una garantía, no una tendencia. |
| Se guarda el **motivo** de la prioridad (embarazo, +65, discapacidad, ayuno), no un booleano | Booleano `prioritario` | Requisito legal: hay que poder demostrar cumplimiento. |
| Toda excepción (general que pasa por encima de un prioritario) se registra con su justificación | No registrar | Sin eso no se puede auditar el cumplimiento normativo. |

**Nota sobre mensajería:** todavía no se decidió nada. Pero quedó claro que la cola de
pacientes **no** es una cola de mensajes: es un orden sobre filas de la base. Un broker
(Kafka, Redis) resuelve transporte y desacople de consumidores, no "quién sigue".

---

### 3. Errores que cometí y qué los causaba

1. **"El TV debe verificar si fue atendido y volver a llamar."**
   *`No atendido` no significa `hay que rellamar`:* significa que el paciente está caminando.
   Rellamar es una acción del operador, limitada a 2 por enunciado. Un cliente que decide
   reanunciar en cada snapshot reanuncia el mismo ticket decenas de veces.

2. **"Un candado sobre la cola, saco el ticket y libero."**
   *Sacar = borrar.* Debe ser transición a `LLAMADO`. Si se borra y falla el envío de la
   respuesta, el paciente desaparece.

3. **"Candado a la cola" (global).** Bloquea boxes de servicios no relacionados. Con 6 boxes
   en hora pico, el sistema se cae solo.

4. **Candado en memoria del proceso.** No sobrevive un reinicio ni funciona con dos
   instancias. Si además se mantiene tomado esperando al operador, un timeout lo deja
   trabado y para todo el laboratorio.

5. **"Los 44 minutos se cuentan desde `llamar al siguiente`."**
   Es el *final* de la espera, no el principio; da siempre 0 y es circular (el número que uso
   para decidir no puede nacer de la decisión). Además, al derivar hay que **copiar**
   `esperando_desde`, no resetearlo, o el paciente nunca llega a los 45 minutos.

6. **"La función que decide vive en el servidor que gestiona las peticiones."**
   Demasiado grueso y peligroso: ahí es donde termina metida dentro del handler HTTP, pegada
   al request y al acceso a datos. Se retoma la próxima sesión.

---

### 4. Puntos a reforzar

- **Capas de verdad.** Qué significa "la lógica de negocio desacoplada de la UI y de la
  infraestructura". Diferencia entre handler, caso de uso, entidad y repositorio.
- **Cómo se testea una política de scheduling** sin base de datos ni HTTP.
- **Idempotencia de las acciones del operador.** Doble clic, respuesta perdida, reintento:
  ¿es un rellamado o un segundo llamado?
- **Transacciones y aislamiento.** Por qué el `UPDATE` condicional es atómico y qué nivel de
  aislamiento se necesita.
- **Transporte en tiempo real** con cuatro consumidores distintos (TV, boxes, tótem,
  supervisión): cuándo alcanza polling, cuándo hace falta push, y cómo se hace *reconnect*
  sin perder estado.
- **Testing de casos borde.** Carrera entre boxes, reconexión del TV, reactivación de ausente
  dentro de los 10 minutos, cierre de jornada con tickets adentro.
- **Qué es un broker y cuándo NO usarlo.** No meter mensajería donde el problema es de datos.

---

### 5. Próxima sesión

1. Terminar el diseño por capas: **dónde vive la política de scheduling** y qué necesita como
   entrada para poder testearse aislada.
2. Idempotencia de las acciones del operador.
3. Ciclo de vida completo del ticket como máquina de estados (incluye `AUSENTE` y
   reactivación de 10 minutos).
4. Elección del transporte realtime, con el criterio de reconexión ya definido.

**Objetivo de fondo:** poder justificar cada decisión arquitectónica del sistema frente a
alguien que pregunte "¿y por qué así?".

---

## Sesión 2 — 2026-11-28 — Política de scheduling: capas, aging y primeros tests

**Objetivo:** cerrar el punto 1 pendiente de la Sesión 1: sacar la decisión "a quién llamar" del handler HTTP, dejarla como regla pura y testeable sin base ni HTTP, y escribir los primeros tests en Go.

**Stack declarado (diferido):** backend en Go y frontend en Next.js + TypeScript, para reforzar esos lenguajes. No se decidieron frameworks ni estructura de carpetas todavía.

---

### 1. Conceptos que aprendí

**Handler ≠ política.** El handler es un traductor (bytes → comando → bytes). Si la regla de orden vive ahí, depende del request y del reloj, no se puede testear sin servidor, y se duplica si cambia el transporte. Analogía: el mozo toma el pedido, la cocina decide la receta, la despensa guarda los ingredientes.

**Decisión pura vs. concreción atómica.** La política decide QUÉ ticket sigue y devuelve un candidato (o nil). El caso de uso es el que lo reclama con `UPDATE ... WHERE id = ... AND estado = 'EN_ESPERA'`. El retorno de la política es un snapshot, no una reserva: entre la lectura y el UPDATE otro box puede ganar la carrera.

**Selección mecánica vs. decisión de negocio.** La base hace lo barato y objetivo: el candidato más viejo de cada categoría (`ORDER BY esperando_desde ... LIMIT 1`). La política hace lo que requiere criterio: comparar esas dos cabezas. Si el criterio se mete en el ORDER BY, la regla deja de existir como unidad testeable.

**Aging (envejecimiento).** Viene de planificación de procesos en sistemas operativos: un trabajo de baja prioridad gana prioridad efectiva cuanto más espera, para evitar starvation. La categoría no cambia; cambia su peso en el momento de decidir. Punto crítico: si a todos se les suma lo mismo por esperar, la diferencia nunca se cierra. El envejecimiento tiene que ser diferencial.

**Puntaje = unidad común.** No se puede comparar "prioritario" contra "28 minutos". El puntaje convierte categoría + tiempo a una sola magnitud (minutos-equivalentes). No es una columna de la base ni estado persistido: se calcula adentro de la función, se usa y se descarta.

**Tope vs. objetivo.** Un deadline puro (el general gana siempre a los 45) cumple la norma pero convierte el techo en el tiempo esperado, y puede invertir el starvation hacia los prioritarios. El aging reparte mejor antes del límite; el tier duro de 45 queda como garantía legal.

**Dos fuentes de verdad.** Tener la categoría en el parámetro (`general`, `prioritario`) y en el campo `Prioridad` es redundante. Un dato que no se verifica no significa nada: conviene validar en la frontera (al insertar y al hidratar desde la BD), no dentro de la regla pura, porque la firma actual no tiene canal para devolver error.

**Go: time.Time y time.Duration.** `EsperandoDesde` es un instante, no minutos. `t.Minute()` devuelve el campo minuto del reloj (0-59), NO el tiempo transcurrido: el valor puede bajar aunque el paciente espere más. La espera correcta es `ahora.Sub(esperandoDesde)`, que devuelve `time.Duration` (nanosegundos). `ahora.Add(-45 * time.Minute)` construye un instante en el pasado; `45 * time.Minute` es una duración. Sumar `20` a una Duration son 20 nanosegundos, no 20 minutos: hay que escribir `20 * time.Minute`. En Go los parámetros no usados NO dan error de compilación, así que un `ahora` muerto pasa desapercibido.

**Tests table-driven.** Una tabla de casos con `nombre`, entradas y `esperado`, recorrida con `t.Run` para tener subtests. `t.Errorf` marca fallo y sigue; `fmt.Errorf` solo construye un error y lo descarta. Los verbos de formato son `%s` / `%d` / `%v` (con porcentaje), no `&s`. `go test` corre `go vet` antes de ejecutar, y falla el build si hay argumentos sin verbos de formato. Requiere `go.mod` en la raíz (`go mod init gestor-turnos`).

**Determinismo en tests.** `time.Now()` es un blanco móvil: dos llamadas difieren en nanosegundos, así que las esperas dan casi cero y el mismo test puede fallar a veces. Se usa un `fechaFija` con `time.Date(...)` y se derivan las esperas con `Add(-N * time.Minute)`.

---

### 2. Decisiones tomadas y por qué

| Decisión | Alternativa descartada | Motivo |
|---|---|---|
| Política pura separada del handler | Regla dentro del handler HTTP | Testabilidad y desacople del framework y del transporte. |
| La BD entrega dos cabezas (la más vieja de cada categoría) | La BD ordena por prioridad efectiva | Si el criterio vive en el ORDER BY, la regla no se puede testear ni tiene un único dueño. |
| Aging + tier duro de 45 | Deadline puro (45 como único criterio) | El deadline convierte el techo en objetivo y puede starvear a los prioritarios con flujo sostenido de generales. |
| Descartado el 2:1 proporcional | Alternar 2 prioritarios por 1 general | Necesita un contador global compartido por 6 boxes; con concurrentes dos pueden leer el mismo estado y decidir igual. |
| Base 20 min, ritmo general 2, ritmo prioritario 1 | Ritmos iguales | Con ritmos iguales la distancia nunca se cierra y el aging no ocurre. |
| Empate a favor del prioritario | Empate a favor del general | Criterio de justicia elegido; queda registrado aunque con Duration en nanosegundos el empate exacto es casi imposible. |
| Validar la categoría en la frontera (constructor) | Validar dentro de `scheduler` con `error` | La firma actual devuelve solo `*Paciente`; agregar error cambia todo el llamador, y la validación dentro de la regla repite una garantía del llamador. |
| El `ahora` viaja como parámetro | `time.Now()` dentro de la función | Determinismo: mismo input, mismo output, siempre. |
| Devolver el `Paciente` entero | Devolver solo el id o solo la categoría | La salida ES la decisión; el llamador necesita el id para el UPDATE atómico; el objeto es un snapshot, no una reserva. |

---

### 3. Errores que cometí y qué los causaba

1. **`EsperandoDesde.Minute()` como tiempo esperado.** Es el campo minuto del reloj (0-59), no la diferencia. Un paciente que entró a las 08:58 y se llama a las 09:01 da 1, y uno que entró a las 08:01 y se llama a las 08:58 también da 1. Por eso `ahora` parecía no tener lugar.
2. **El comment `// en minutos` sobre un `time.Time`.** Indujo al error anterior. Un comment falso es peor que ninguno.
3. **`+ 20` sobre una `Duration`.** Son 20 nanosegundos. El base efectivo quedaba en casi cero y el general ganaba aunque hubiera un prioritario esperando.
4. **Puntaje sin unidad común.** Comparar categoría contra duración directamente no es posible; el `* time.Minute` convierte el base a la misma unidad.
5. **`time.Now()` en los datos de test.** Las esperas salían en nanosegundos y los casos de 45 min nunca se disparaban.
6. **`fmt.Errorf` en vez de `t.Errorf`, y `&s` en vez de `%s`.** El primero construye un error y no falla el test; el segundo no es un verbo de formato. `go test` falla en `vet` antes de ejecutar.
7. **Esperado derivado del mismo campo comparado** (`esperado: generalCorto.Prioridad`). La expectativa debe ser independiente: escribir `"G"` o `"P"` a seco.
8. **Expectativas contradictorias con el nombre del caso** y esperas negativas por usar `Add(+5 * time.Minute)` con un `ahora` anterior. Los tests pasaron pero dos casos siguen mal nombrados.

---

### 4. Estado real del código al cierre

`funcion.go` contiene `scheduler(general, prioritario *Paciente, ahora time.Time) *Paciente` con: casos `nil`, tier duro de 45, `puntajeGeneral = espera * 2`, `puntajePrioritario = espera + 20 * time.Minute`, empate a favor del prioritario.

`funcion_test.go` tiene 4 casos table-driven con `fechaFija` y `go test -v ./...` pasa (4/4).

**Observaciones pendientes (no corregidas):** los casos 3 y 4 se llaman "Prioritario gana en límite/exceso" pero esperan `"G"`; `esperado` se sigue derivando de `.Prioridad` en vez de un literal; el campo `Prioridad` todavía no se valida con constructor; el comment `// instante finito de tiempo` es incorrecto; no hay casos `nil`, de empate ni de reloj futuro.

---

### 5. Próxima sesión

1. Corregir los nombres y los esperados de los casos 3 y 4, y escribir los esperados como literales.
2. Agregar casos borde: ambos `nil`, solo un candidato, empate exacto, `EsperandoDesde` en el futuro.
3. Implementar el constructor en la frontera que rechace categoría inválida y fecha futura; tipar la categoría con constantes en vez de `string`.
4. Mover la política a su propia capa y conectar el caso de uso con el `UPDATE ... WHERE estado = 'EN_ESPERA'`.
5. Retomar los puntos 2, 3 y 4 pendientes de la Sesión 1: idempotencia del operador, ciclo de vida con `AUSENTE`, y transporte realtime.

**Objetivo de fondo:** poder justificar cada decisión frente a alguien que pregunte "¿y por qué así?".
