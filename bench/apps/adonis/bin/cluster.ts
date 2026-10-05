/*
|--------------------------------------------------------------------------
| Cluster entrypoint
|--------------------------------------------------------------------------
|
| Runs "bin/server.js", Adonis's own HTTP server, in $WORKERS processes
| that share the port through Node's cluster module. AdonisJS's deployment
| guide for v6 ("Starting the production server") recommends PM2 for that,
| with `instances: 'max'` and `exec_mode: 'cluster'`; v7's starts one
| `node bin/server.js` and names no process manager. PM2's cluster mode is
| Node's cluster module, which this file uses with nothing installed
| globally: this process accepts the connections and hands them to the
| workers in turn, Node's default everywhere but Windows. As PM2's
| `autorestart`, a worker that dies after it was serving is started again.
|
*/

import cluster, { type Worker } from 'node:cluster'
import { availableParallelism } from 'node:os'
import { fileURLToPath } from 'node:url'

/**
 * One worker per CPU unless WORKERS says otherwise, as PM2's
 * `instances: 'max'`.
 */
const count = Number(process.env.WORKERS) || availableParallelism()

const workers = new Set<Worker>()
const serving = new Set<Worker>()
let stopping = false

cluster.setupPrimary({ exec: fileURLToPath(new URL('./server.js', import.meta.url)) })

function start() {
  const worker = cluster.fork()
  worker.on('error', (error) => console.error(`worker ${worker.process.pid}: ${error.message}`))
  workers.add(worker)
}

/**
 * Stops the workers, and this process once they're all gone. Each worker
 * is sent the signal, for one that came to this process alone rather than
 * to the process group, and Adonis's handler of SIGTERM closes its server.
 * Each is disconnected too, as a worker lives on, its server closed, while
 * its channel to this process is open.
 */
function stop(signal: NodeJS.Signals) {
  stopping = true
  for (const worker of workers) {
    worker.process.kill(signal)
    if (worker.isConnected()) worker.disconnect()
  }
}

/**
 * Stops the cluster with an error, for workers that failed as they booted,
 * as others started in their place would fail the same way.
 */
function fail(reason: string) {
  console.error(reason)
  process.exitCode = 1
  stop('SIGTERM')
}

/**
 * A worker whose server can't start, as on a port that's taken, says why
 * but lives on, held by its channel to this process: so the cluster gives
 * up if its workers aren't all serving soon after it starts them.
 */
const booting = setTimeout(() => {
  fail(`${serving.size} of ${count} workers serving after 20 seconds`)
}, 20_000)

cluster.on('listening', (worker, address) => {
  serving.add(worker)
  if (serving.size === count && !stopping) {
    clearTimeout(booting)
    console.log(`${count} workers listening on ${address.address}:${address.port}`)
  }
})

cluster.on('exit', (worker, code, signal) => {
  workers.delete(worker)
  const wasServing = serving.delete(worker)
  if (stopping) {
    if (workers.size === 0) process.exit()
    return
  }

  const exited = `worker ${worker.process.pid} exited with ${signal ?? code}`
  if (wasServing) {
    console.error(`${exited}, starting another`)
    start()
    return
  }

  fail(`${exited} before it served`)
  if (workers.size === 0) process.exit()
})

process.on('SIGTERM', () => stop('SIGTERM'))
process.on('SIGINT', () => stop('SIGINT'))

for (let i = 0; i < count; i++) start()
