#!/usr/bin/env python3

import json
import re
import matplotlib.pyplot as plt

snapshots = []

with open("./resource-usage-log.txt") as file:
  current_snapshot = None
  for line in file:
    # The lines might contain the reset characters before the actual JSON.
    # This means that the entire resources for the current time have been
    # written out, so we add the latest snapshot to our list and continue
    # reading the next entries.
    first_backet = line.find("{")
    if first_backet == -1:
      continue

    if first_backet != 0:
      if current_snapshot is not None:
        snapshots.append(current_snapshot)

      current_snapshot = []
      line = line[first_backet:]

    current_snapshot.append(json.loads(line))

def parse_percentage(string):
  return float(string.strip('%'))

units = {"B": 1, "kB": 10**3, "MB": 10**6, "GB": 10**9, "TB": 10**12,
         "KiB": 2**10, "MiB": 2**20, "GiB": 2**30, "TiB": 2**40}

def parse_byte_size(size):
  number, unit = re.findall(r'([0-9\.]+)([A-Za-z]+)', size)[0]
  return int(float(number)*units[unit])

def parse_two_bytes(string):
  str1, str2 = string.split("/")
  return parse_byte_size(str1), parse_byte_size(str2)

s3_cpu = []
s3_mem = []
tusd_cpu = []
tusd_mem = []
tusd_net = []
uploader_cpu = []
uploader_mem = []
uploader_net = []
timestamp = []

for (i, snapshot) in enumerate(snapshots):
  a_s3_cpu = None
  a_s3_mem = None
  a_tusd_cpu = None
  a_tusd_mem = None
  a_tusd_net = None
  a_uploader_cpu = None
  a_uploader_mem = None
  a_uploader_net = None

  for entry in snapshot:
    if entry["Name"] == "load-tests-tusd-1":
      a_tusd_cpu = parse_percentage(entry["CPUPerc"])
      a_tusd_mem = parse_two_bytes(entry["MemUsage"])[0]
      a_tusd_net = parse_two_bytes(entry["NetIO"])[0]
    elif entry["Name"] == "load-tests-s3-1":
      a_s3_cpu = parse_percentage(entry["CPUPerc"])
      a_s3_mem = parse_two_bytes(entry["MemUsage"])[0]
    elif entry["Name"] == "load-tests-uploader-1":
      a_uploader_cpu = parse_percentage(entry["CPUPerc"])
      a_uploader_mem = parse_two_bytes(entry["MemUsage"])[0]
      a_uploader_net = parse_two_bytes(entry["NetIO"])[1]

  s3_cpu.append(a_s3_cpu)
  s3_mem.append(a_s3_mem)
  tusd_cpu.append(a_tusd_cpu)
  tusd_mem.append(a_tusd_mem)
  tusd_net.append(a_tusd_net)
  uploader_cpu.append(a_uploader_cpu)
  uploader_mem.append(a_uploader_mem)
  uploader_net.append(a_uploader_net)

  # The docker stats command is hard coded to output stats every 500ms:
  # https://github.com/docker/cli/blob/81c68913e4c2cb058b5a9fd5972e2989d9915b2c/cli/command/container/stats.go#L223
  timestamp.append(0.5 * i)

fig, axs = plt.subplots(3, 3, sharex=True, sharey='row')
axs[0, 0].plot(timestamp, tusd_cpu)
axs[0, 0].set_title('tusd CPU percentage')
axs[0, 0].set(ylabel='CPU perc', xlabel='time')
axs[0, 1].plot(timestamp, s3_cpu)
axs[0, 1].set_title('s3 CPU percentage')
axs[0, 1].set(ylabel='CPU perc', xlabel='time')
axs[0, 2].plot(timestamp, uploader_cpu)
axs[0, 2].set_title('uploader CPU percentage')
axs[0, 2].set(ylabel='CPU perc', xlabel='time')

axs[1, 0].plot(timestamp, tusd_mem)
axs[1, 0].set_title('tusd memory usage')
axs[1, 0].set(ylabel='mem perc', xlabel='time')
axs[1, 1].plot(timestamp, s3_mem)
axs[1, 1].set_title('s3 memory usage')
axs[1, 1].set(ylabel='mem perc', xlabel='time')
axs[1, 2].plot(timestamp, uploader_mem)
axs[1, 2].set_title('uploader memory usage')
axs[1, 2].set(ylabel='mem perc', xlabel='time')

axs[2, 0].plot(timestamp, tusd_net)
axs[2, 0].set_title('tusd network input')
axs[2, 0].set(ylabel='total volume', xlabel='time')
axs[2, 1].axis('off')
axs[2, 2].plot(timestamp, uploader_net)
axs[2, 2].set_title('uploader network output')
axs[2, 2].set(ylabel='total volume', xlabel='time')


# Hide x labels and tick labels for top plots and y ticks for right plots.
for ax in axs.flat:
    ax.label_outer()

plt.show()
