# Enunciado del Problema: Sistema de Gestión de Turnos y Atención - Laboratorio Clínico

## 1. Contexto y Necesidad del Negocio
Somos un laboratorio de análisis clínicos con alto flujo diario de pacientes en sala de espera. Actualmente, la recepción entrega números de papel de forma manual y los extraccionistas o recepcionistas llaman a los pacientes a viva voz o mediante un tablero numérico básico que no distingue motivos de visita ni prioridades.

Necesitamos un sistema web integral para ordenar el flujo de pacientes desde que ingresan al laboratorio hasta que finalizan su atención, reduciendo los tiempos de espera percibidos, garantizando el cumplimiento legal de atención prioritaria y dando visibilidad en tiempo real tanto al personal como a los pacientes.

## 2. Tipos de Atención (Servicios)
El laboratorio cuenta con distintos circuitos de atención que no siempre comparten los mismos puestos físicos:
1. **Recepción / Admisión (Autorizaciones y Presupuestos):** Pacientes que llegan por primera vez, deben validar órdenes médicas, obras sociales o pedir presupuestos antes de pasar a extracción.
2. **Extracción de Sangre / Toma de Muestras:** Pacientes ya admitidos (o con turno web pre-admitido) listos para pasar a los boxes de extracción.
3. **Entrega de Muestras (Orina / Materia Fecal):** Circuito rápido para pacientes que solo vienen a dejar un frasco ya recolectado y no requieren extracción.
4. **Retiro de Resultados:** Pacientes que buscan estudios impresos o consultas administrativas rápidas.

## 3. Reglas de Prioridad y Justicia en la Cola
Por normativa y criterio médico, existen dos niveles de atención dentro de cada servicio:
* **Atención Prioritaria:** Embarazadas, adultos mayores (+65 años), personas con discapacidad o movilidad reducida, y pacientes en ayuno prolongado por estudios específicos (ej. curva de glucosa).
* **Atención General:** Resto de los pacientes.

**Requerimiento crítico de negocio:** No podemos atender *exclusivamente* a la cola prioritaria hasta vaciarla si eso provoca que un paciente general espere más de 45 minutos. El sistema debe aplicar un criterio de balanceo (ej. proporción 2 prioritarios cada 1 general, o aumento dinámico de prioridad por tiempo de espera) para evitar que los pacientes generales queden bloqueados indefinidamente.

## 4. Actores y Puntos de Contacto del Sistema

### A. Tótem de Autogestión (Ingreso del Paciente)
* Ubicado en la entrada del laboratorio (pantalla táctil).
* El paciente ingresa su DNI (o escanea su documento/turno previo), selecciona el trámite a realizar e indica si corresponde a **Atención Prioritaria**.
* El sistema emite un ticket identificatorio con prefijo según servicio y prioridad (ej. `EP-004` para Extracción Prioritaria, `EG-019` para Extracción General) y le muestra cuántas personas tiene adelante en su circuito.

### B. Puestos de Atención (Recepcionistas y Extraccionistas)
* Cada empleado inicia sesión y se asigna a un **Box o Escritorio físico** (ej. *Box 1*, *Box 2*, *Mostrador A*), indicando qué tipos de atención puede resolver ese puesto en ese turno.
* **Acciones del operador:**
  * **Llamar siguiente:** El sistema decide cuál es el próximo ticket óptimo según las reglas de prioridad y los servicios habilitados para ese box.
  * **Rellamar:** Si el paciente no se acerca tras el primer llamado, el operador puede volver a disparar la alerta sonora/visual (máximo 2 rellamados).
  * **Marcar Ausente:** Si tras los rellamados no aparece, el ticket pasa a estado `AUSENTE`.
  * **Iniciar / Finalizar Atención:** Registra los tiempos reales de atención para métricas.
  * **Derivar Paciente:** Un recepcionista que termina de admitir a un paciente puede derivarlo directamente a la cola de *Extracción de Sangre* manteniendo su antigüedad o prioridad original, sin que el paciente tenga que volver al tótem.

### C. Pantalla de Sala de Espera (Cartelería Digital)
* Televisor visible para todos los pacientes en la sala.
* Muestra de forma clara y grande el código del ticket llamado y el Box asignado, acompañado de un sonido de alerta.
* Mantiene un historial visible de los últimos 4 o 5 llamados.
* Muestra el tiempo estimado de espera actualizado para cada tipo de servicio.

### D. Panel de Supervisión / Coordinación
* Vista para el jefe de laboratorio donde pueda ver en vivo: cuántos pacientes hay esperando por servicio, tiempo promedio de espera actual, boxes activos/pausados y alertas si algún paciente supera el umbral máximo de espera tolerado.

## 5. Casos Borde y Restricciones Operativas
1. **Concurrencia:** En horas pico (7:00 a 9:30 AM) hay hasta 6 boxes llamando pacientes simultáneamente. Bajo ninguna circunstancia dos boxes pueden llamar al mismo paciente al mismo tiempo.
2. **Reincorporación de Ausentes:** Si un paciente fue marcado como `AUSENTE` porque estaba en el baño y se presenta en recepción dentro de los 10 minutos siguientes, el recepcionista debe poder reactivar su ticket para que sea llamado a la brevedad sin perder todo su turno.
3. **Resiliencia de Pantalla:** Si la conexión de red del televisor de la sala de espera sufre un microcorte, al reconectarse debe sincronizar inmediatamente el estado actual de los llamados sin requerir intervención manual.
4. **Cierre de Jornada:** Al finalizar el día, el sistema debe permitir cerrar la recepción de nuevos tickets mientras se termina de atender a quienes ya están adentro, reiniciando la numeración al día siguiente pero conservando el historial estadístico.

## 6. Objetivo Técnico del Proyecto
Queremos construir una solución robusta, mantenible y bien estructurada donde la lógica de negocio (reglas de asignación, prioridades, ciclo de vida del ticket y derivaciones) esté completamente desacoplada de la interfaz de usuario y de los detalles de infraestructura. El sistema debe soportar actualizaciones en tiempo real y garantizar la integridad de los datos ante operaciones concurrentes.
