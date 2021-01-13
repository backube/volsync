.. These are the descriptions for the common volume handling options

accessModes
   When using a copyMethod of Clone or Snapshot, this field allows overriding
   the access modes for the point-in-time (PiT) volume. The default is to use the
   access modes from the source PVC.
capacity
   When using a copyMethod of Clone or Snapshot, this allows overriding the
   capacity of the PiT volume. The default is to use the capacity of the source
   volume.
copyMethod
   This specifies the method used to create a PiT copy of the source volume.
   Valid values are:

   - **Clone** - Create a new volume by cloning the source PVC (i.e., use the
     source PVC as the volumeSource for the new volume.
   - **None** - Do no create a PiT copy. The Scribe data mover will directly use
     the source PVC.
   - **Snapshot** - Create a VolumeSnapshot of the source PVC, then use that
     snapshot to create the new volume. This option should be used for CSI
     drivers that support snapshots but not cloning.
storageClassName
   This specifies the name of the StorageClass to use when creating the PiT
   volume. The default is to use the same StorageClass as the source volume.
volumeSnapshotClassName
   When using a copyMethod of Snapshot, this specifies the name of the
   VolumeSnapshotClass to use. If not specified, the cluster default will be
   used.
