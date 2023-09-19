========
Triggers
========

There are three types of triggers in volsync:

1. Always - no trigger, always run.
2. Schedule - defined by a cronspec.
3. Manual - request to trigger once.

See the sections below with details on each trigger type.


Always
======

.. code:: yaml

   spec:
     trigger: {}


This option is set either by omitting the trigger field completely or by setting it to empty object.
In both cases the effect is the same - keep replicating all the time.

When using Rsync-based replication, the destination should be set to always-listen
for incoming replications from the source. Therefore, the default configuration for rsync destination
is with no trigger, which keeps waiting for the next trigger by the source to connect.

In this case ``status.nextSyncTime`` will not be set,
but ``status.lastSyncTime`` will be set at the end of every replication.


Schedule
========

.. code:: yaml

   spec:
     trigger:
       schedule: "*/6 * * * *"


The synchronization schedule, ``.spec.trigger.schedule``, is defined by a
`cronspec <https://en.wikipedia.org/wiki/Cron#Overview>`_, making the schedule
very flexible. Both intervals (shown above) as well as specific times and/or
days can be specified.

In this case ``status.nextSyncTime`` will be set to the next schedule time based on the cronspec,
and ``status.lastSyncTime`` will be set at the end of every replication.


Manual
======

.. code:: yaml

   spec:
     trigger:
       manual: my-manual-id-1

Manual trigger is used for running one replication and wait for it to complete. This is useful to control the replication schedule from an external automation (for example using quiesce for live migration).

To use the manual trigger choose a string value and set it in ``spec.trigger.manual``
which will start a replication. Once replication completes, ``status.lastManualSync``
will be set to the same string value. As long as these two values are the same
there will be no trigger, and the replication will remain paused,
until further updates to the trigger spec.

After setting the manual trigger in spec, the user should watch for ``status.lastManualSync``
and wait for it to have the expected value, which means that the manual trigger completed.
If needed, the user can then continue to update ``spec.trigger.manual`` to a new value
in order to trigger another replication.

Something to keep in mind when using manual trigger - the update of ``spec.trigger.manual`` by itself
does not interrupt a running replication, and ``status.lastManualSync`` will simply be set to the value
from the spec when the current replication completes. This means that to make sure we know when the
replication started, and that it includes the latest data, it is recommended to wait until
``status.lastManualSync`` equals to ``spec.trigger.manual`` before setting to a new value.

In this case ``status.nextSyncTime`` will not be set, but ``status.lastSyncTime`` will be set at the end of every replication.

Here is an example of how to use manual trigger to run two replications:

.. code:: bash

   MANUAL=first
   SOURCE=source1

   # create source replication with first manual trigger (will start immediately)
   kubectl create -f - <<EOF
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: $SOURCE
   spec:
     trigger:
       manual: $MANUAL
     ...
   EOF

   # waiting for first trigger to complete...
   while [ "$LAST_MANUAL_SYNC" != "$MANUAL" ]
     do
       sleep 1
       LAST_MANUAL_SYNC=$(kubectl get replicationsource $SOURCE --template={{.status.lastManualSync}})
       echo " - LAST_MANUAL_SYNC: $LAST_MANUAL_SYNC"
     done

   # set a second manual trigger
   MANUAL=second
   kubectl patch replicationsources $SOURCE --type merge -p '{"spec":{"trigger":{"manual":"'$MANUAL'"}}}'

   # waiting for second trigger to complete...
   while [ "$LAST_MANUAL_SYNC" != "$MANUAL" ]
     do
       sleep 1
       LAST_MANUAL_SYNC=$(kubectl get replicationsource $SOURCE --template={{.status.lastManualSync}})
       echo " - LAST_MANUAL_SYNC: $LAST_MANUAL_SYNC"
     done

   # after second trigger is done we delete the replication...
   kubectl delete replicationsources $SOURCE
