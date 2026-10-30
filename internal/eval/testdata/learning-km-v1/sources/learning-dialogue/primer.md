# Fault alerts: a fictional worked example

This is an invented device-inspection example, not observed field data.
Out of 1,000 devices, 20 are faulty and 980 are sound. A detector flags 80% of
faulty devices and incorrectly flags 5% of sound devices. It produces 16 true
alerts and 49 false alerts, for 65 alerts in total. Thus 16/65, about 24.62%,
of alerts correspond to a fault. The 80% detection rate has faulty devices as
its denominator; the probability a flagged device is faulty has all alerts as
its denominator. Swapping these denominators causes the mistake.

A useful learning approach is to count the two groups, count alerts within each,
then combine the alert counts. Ask the learner to explain the denominator before
showing a similar problem. A practice problem can change the base rate while
holding detector rates fixed; do not imply that this changes the detector itself.
