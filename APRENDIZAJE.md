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
