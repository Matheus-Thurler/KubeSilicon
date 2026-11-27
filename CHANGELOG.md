# Changelog

## [0.5.1](https://github.com/Matheus-Thurler/KubeSilicon/compare/v0.5.0...v0.5.1) (2026-11-27)


### Bug Fixes

* **kfp:** tell the step when the log tail stops at 4 KiB ([4f61038](https://github.com/Matheus-Thurler/KubeSilicon/commit/4f610381ea36f48a31772cb83af560b05972628d))
* **kfp:** tell the step when the log tail stops at 4 KiB ([8cb39d3](https://github.com/Matheus-Thurler/KubeSilicon/commit/8cb39d387ae05b5c00c5521b4dba13cbbb31eb60))
* one daemon per Mac, or two guests open the same disk ([0cbe763](https://github.com/Matheus-Thurler/KubeSilicon/commit/0cbe763e82a9d93aeb3ccfe660ef6eb63082d019))
* one daemon per Mac, or two guests open the same disk ([3e931f0](https://github.com/Matheus-Thurler/KubeSilicon/commit/3e931f00d1ee1276e407d542d064a945c1135119))
* serial console socket with long VM paths ([23522fc](https://github.com/Matheus-Thurler/KubeSilicon/commit/23522fc6c97e5529b71d2a0a67ef8a5554654b00))
* serial console socket with long VM paths ([f4f3172](https://github.com/Matheus-Thurler/KubeSilicon/commit/f4f3172bd7f2f9d4535cad71af6385a3b4164e43))
* the second status write of a pass is compared with the old one ([fb61e56](https://github.com/Matheus-Thurler/KubeSilicon/commit/fb61e56e83181f03329d59521f2ba44163f0ad1d))
* **vm:** a crashed VM stays stopped after a daemon restart ([53131f3](https://github.com/Matheus-Thurler/KubeSilicon/commit/53131f337d982a4b9d98a1a99ffb96a1493e63ee))
* **vm:** restarts, status writes and DNS after a daemon restart ([0792122](https://github.com/Matheus-Thurler/KubeSilicon/commit/079212234616a447956f4688d4d1dd5da715e76c))
* **vm:** the Service comes back when it goes missing ([a2029df](https://github.com/Matheus-Thurler/KubeSilicon/commit/a2029dfeaa1ac60d1d42a62128741ff268827b5d))

## [0.5.0](https://github.com/Matheus-Thurler/KubeSilicon/compare/v0.4.0...v0.5.0) (2026-11-26)


### Features

* **job:** startTime, completionTime and exitCode in status ([a374c37](https://github.com/Matheus-Thurler/KubeSilicon/commit/a374c37606e24018bf005ab8dad774ee2adefccf))
* **job:** the CRD rejects an empty job and changes to a created job ([66f5477](https://github.com/Matheus-Thurler/KubeSilicon/commit/66f5477c9a59dc6a58851dfb3aa4b031fff87463))
* **vm:** Ready condition, restartPolicy, nested virtualization and start/stop/restart ([6a53c80](https://github.com/Matheus-Thurler/KubeSilicon/commit/6a53c80126fd3e5d0a86547554c9f1bc8855a385))
* **vm:** Ready condition, restartPolicy, nested virtualization, parallel workers and start/stop/restart ([30767e0](https://github.com/Matheus-Thurler/KubeSilicon/commit/30767e0980855feeee424117cd84cf8d6b163bba))


### Bug Fixes

* **cli:** logs uses the namespace of the context ([22ebfd8](https://github.com/Matheus-Thurler/KubeSilicon/commit/22ebfd8f816f50aba8bf43a11b6e5d5119d9e9a3))
* **cli:** start brings back a VM whose guest shut itself down ([27d3edd](https://github.com/Matheus-Thurler/KubeSilicon/commit/27d3edd0014fd43b21b862f46440fd3ef8fe37b1))
* **job:** a script that exits 130 is Failed, not Cancelled ([b0e83a8](https://github.com/Matheus-Thurler/KubeSilicon/commit/b0e83a8338aa479f62d647541dea764d601b78d8))
* **job:** kill the old process group before a retry ([e4d0795](https://github.com/Matheus-Thurler/KubeSilicon/commit/e4d07959f39af5bd8ef57bc783066b8a12488ba5))
* **job:** make the terminated flag safe across goroutines ([9939a57](https://github.com/Matheus-Thurler/KubeSilicon/commit/9939a5793b20860f4d8e0829d7e48dc389fe66b8))
* **job:** redact signed URLs, report SIGTERM during transfers, ignore process groups from before a reboot ([45b2fab](https://github.com/Matheus-Thurler/KubeSilicon/commit/45b2fab9608eb6f1fc498f1cea20ceddf11db2be))
* **job:** robustness, tighter sandbox and API validation ([c7cc71e](https://github.com/Matheus-Thurler/KubeSilicon/commit/c7cc71ee75e4c50c87926665e69f08e55e1292c1))
* **job:** the sandbox blocks ways out through other processes ([455b0b3](https://github.com/Matheus-Thurler/KubeSilicon/commit/455b0b31a45ed9ca1dc19a79082a2360ee67055c))
* **job:** the timeout also covers downloads and uploads ([a7bf9f8](https://github.com/Matheus-Thurler/KubeSilicon/commit/a7bf9f8e7c77e07001efbfbdbe3ad665852dd336))

## [0.4.0](https://github.com/Matheus-Thurler/KubeSilicon/compare/v0.3.2...v0.4.0) (2026-11-11)


### Features

* **job:** run jobs in parallel when their memory fits ([05084d3](https://github.com/Matheus-Thurler/KubeSilicon/commit/05084d3355b8ebf798a404a720f13114c3cc9d3a))
* **job:** run jobs in parallel when their memory fits ([371ae9c](https://github.com/Matheus-Thurler/KubeSilicon/commit/371ae9c031637c48e2a99259686abcd2ce9dde65))


### Bug Fixes

* **job:** drop the stale memory sample on retry and the empty peak line ([52cd96c](https://github.com/Matheus-Thurler/KubeSilicon/commit/52cd96c2eedd04bfb6a754ce2b2293dee9f4350f))

## [0.3.2](https://github.com/Matheus-Thurler/KubeSilicon/compare/v0.3.1...v0.3.2) (2026-11-11)


### Bug Fixes

* close two ways a cluster object could reach files or code on the host ([bed01c0](https://github.com/Matheus-Thurler/KubeSilicon/commit/bed01c0b21d201325537a0419d83d932b45e17aa))
* **image:** reject disk sources that read files of the host ([796e6fb](https://github.com/Matheus-Thurler/KubeSilicon/commit/796e6fbba5e8706d2c26ec99b4b4183dcb85c058))
* **job:** validate python and dependencies before uv runs them ([77dcaf1](https://github.com/Matheus-Thurler/KubeSilicon/commit/77dcaf174da3952e63c760f38fa229c5416c8a65))

## [0.3.1](https://github.com/Matheus-Thurler/KubeSilicon/compare/v0.3.0...v0.3.1) (2026-11-10)


### Bug Fixes

* give every Fashion-MNIST run its own bucket prefix ([faab3f5](https://github.com/Matheus-Thurler/KubeSilicon/commit/faab3f5253e0eeeb2d8530fbb2e0f7755447f5b2))
* give every Fashion-MNIST run its own bucket prefix ([c80daab](https://github.com/Matheus-Thurler/KubeSilicon/commit/c80daabb390576ac19b26f00f4f10530d8757852))

## [0.3.0](https://github.com/Matheus-Thurler/KubeSilicon/compare/v0.2.0...v0.3.0) (2026-11-09)


### Features

* Fashion-MNIST pipeline with data, GPU training and evaluation ([5265fa0](https://github.com/Matheus-Thurler/KubeSilicon/commit/5265fa0a00c00388685c22e4ca322fbd54b12e94))
* Fashion-MNIST pipeline with data, GPU training and evaluation ([1271984](https://github.com/Matheus-Thurler/KubeSilicon/commit/12719846f3cd0ff4dbd82ea0e03b2dfe92e02752))

## [0.2.0](https://github.com/Matheus-Thurler/KubeSilicon/compare/v0.1.0...v0.2.0) (2026-11-09)


### Features

* initial import with SiliconVM lifecycle on Apple Silicon ([458f16e](https://github.com/Matheus-Thurler/KubeSilicon/commit/458f16e1fe2baa3ee69a2c506f64bb8f36bd5f33))
* Kubeflow Pipelines integration, status.logTail and kind-kubeflow setup script ([76c025b](https://github.com/Matheus-Thurler/KubeSilicon/commit/76c025b8f29b241c904de9d2008d1c16a0cb8f70))
* kubesilicon client with ip, ssh and serial console ([bbcca7a](https://github.com/Matheus-Thurler/KubeSilicon/commit/bbcca7ae042d341993491a131c37384471f895b0))
* make install, example VM and terminal tutorial tapes ([2875713](https://github.com/Matheus-Thurler/KubeSilicon/commit/2875713b72d17188f97ca2f5fc94a7dcfc4f8137))
* one launcher process per VM and DNS name per VM ([6d7cd70](https://github.com/Matheus-Thurler/KubeSilicon/commit/6d7cd708835ca891a36cddcf8c853221797f3c2d))
* orphan cleanup on start and a LaunchAgent installer ([8d7b144](https://github.com/Matheus-Thurler/KubeSilicon/commit/8d7b1445dbb326da1a1f976b5833cb91692d6ebf))
* production hardening for SiliconJob and the daemon ([68c58bd](https://github.com/Matheus-Thurler/KubeSilicon/commit/68c58bdb08834e23e5521003c6517cd021a0a83b))
* production hardening, retry on interruption, least-privilege daemon, pending timeout ([705a4f0](https://github.com/Matheus-Thurler/KubeSilicon/commit/705a4f09736283eb06a6aaee73774cec7a1515d9))
* SiliconJob, native GPU jobs on the Mac (MPS) with venv, sandbox, queue and artifacts ([8f5be85](https://github.com/Matheus-Thurler/KubeSilicon/commit/8f5be85238571f36ab36f440e23d0b4a79624c74))


### Bug Fixes

* cached disk mode to stop random guest crashes ([c9f9de4](https://github.com/Matheus-Thurler/KubeSilicon/commit/c9f9de4b131b5433bca4caabc3dddcabdd62ec48))
* never rerun a SiliconJob whose status is already final ([eaaf65a](https://github.com/Matheus-Thurler/KubeSilicon/commit/eaaf65a66b858c06f10167d0d00a6b25de7b79af))
* **vm:** keep the NIC MAC across boots ([a9bc9f7](https://github.com/Matheus-Thurler/KubeSilicon/commit/a9bc9f7512e6624e3c306e095aeb6723915b1186))
