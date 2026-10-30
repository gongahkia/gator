**lesson.md**

# How Base Rates Affect Fault Alerts

## Start With the Base Rate

The **base rate** is the proportion of devices that are actually faulty before testing. Even a reasonably accurate detector can produce many false alerts when faults are rare, because it examines far more sound devices than faulty ones.

## Worked Example: 1,000 Devices

Suppose 20 of 1,000 devices are faulty, while 980 are sound. The detector:

- Flags 80% of faulty devices: \(0.80 \times 20 = 16\) true alerts.
- Incorrectly flags 5% of sound devices: \(0.05 \times 980 = 49\) false alerts.

That produces \(16 + 49 = 65\) alerts. Of those, only 16 identify actual faults:

\[
\frac{16}{65} \approx 24.62\%
\]

Thus, receiving an alert does **not** mean there is an 80% chance of a fault. The large group of sound devices generates enough false alerts to outnumber the true alerts. This is a fictional teaching example, not field data ([source/primer.md](source/primer.md)).

## Keep the Denominators Separate

The **80% detection rate** is \(16/20\): its denominator is **all faulty devices**.

The **24.62% fault-among-alerts rate** is \(16/65\): its denominator is **all alerted devices**.

Confusing these denominators causes the base-rate error.

## Check Your Understanding

If 40 of 1,000 devices were faulty while the detector’s 80% detection rate and 5% false-alert rate stayed unchanged, what percentage of all alerts would correspond to real faults?