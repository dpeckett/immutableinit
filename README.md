# matchstick

A `/sbin/init` shim that enables read-only root filesystems for Debian.

## Installation

### From APT

Add my [apt repository](https://github.com/dpeckett/apt.dpeckett.dev?tab=readme-ov-file#usage) to your system.

Then install matchstick:

*Currently packages are only published for Debian 12 (Bookworm).*

```shell
sudo apt update
sudo apt install matchstick
```

### GitHub Releases

Download statically linked binaries from the GitHub releases page: 

[Latest Release](https://github.com/immutos/matchstick/releases/latest)

## Usage

Make sure a `/sbin/init` symlink exists in the root filesystem, and that it points to the matchstick binary.

### Configuration

Matchstick is configured via kernel command line arguments.

* **matchstick.data**: The device to which write operations will be redirected.
* **matchstick.datafstype**: The filesystem type of the data device.

Or, if you don't want to persist changes:

* **matchstick.volatile**: If set to true, the data filesystem will be mounted as a tmpfs, and all changes will be lost on reboot.

And the following optional options are available for advanced users:

* **matchstick.mount**: The mountpoint to be used for the data filesystem, defaults to `/mnt/data`.
* **matchstick.dirs**: A comma-separated list of directories that will be made writable.
* **matchstick.cmd**: The init process to be executed after the filesystem has been mounted, defaults to `/lib/systemd/systemd`.
