.. These are the descriptions for the common volume handling options

accessModes
   When Scribe creates the destination volume, this specifies the accessModes
   for the PVC. The value should be ReadWriteOnce or ReadWriteMany.
capacity
   When Scribe creates the destination volume, this value is used to determine
   its size. This need not match the size of the source volume, but it must be
   large enough to hold the incoming data.
copyMethod
   This specifies how the data should be preserved at the end of each
   synchronization iteration. Valid values are:

   - **None** - Do not create a point-in-time copy of the data.
   - **Snapshot** - Create a VolumeSnapshot at the end of each iteration
destinationPVC
   Instead of having Scribe automatically provision the destination volume
   (using capacity, accessModes, etc.), the name of a pre-existing PVC may be
   specified here.
storageClassName
   When Scribe creates the destination volume, this specifies the name of the
   StorageClass to use. If omitted, the system default StorageClass will be
   used.
volumeSnapshotClassName
   When using a copyMethod of Snapshot, this value specifies the name of the
   VolumeSnapshotClass to use when creating a snapshot.
