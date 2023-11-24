import React, { FC, useState, useEffect, useRef, useCallback } from 'react'
import Box from '@mui/material/Box'
import Grid from '@mui/material/Grid'

import useRouter from '../hooks/useRouter'
import useAccount from '../hooks/useAccount'
import useApi from '../hooks/useApi'
import Divider from '@mui/material/Divider'
import Typography from '@mui/material/Typography'
import FormGroup from '@mui/material/FormGroup'
import FormControlLabel from '@mui/material/FormControlLabel'
import Switch from '@mui/material/Switch'

import Interaction from '../components/session/Interaction'
import Window from '../components/widgets/Window'
import JsonWindowLink from '../components/widgets/JsonWindowLink'
import SessionSummary from '../components/session/SessionSummary'
import SessionHeader from '../components/session/Header'
import RunnerSummary from '../components/session/RunnerSummary'
import SchedulingDecisionSummary from '../components/session/SchedulingDecisionSummary'

import {
  IDashboardData,
  ISession,
  ISessionSummary,
} from '../types'

const START_ACTIVE = false

const Dashboard: FC = () => {
  const account = useAccount()
  const router = useRouter()
  const api = useApi()

  const activeRef = useRef(START_ACTIVE)

  const [ viewingSession, setViewingSession ] = useState<ISession>()
  const [ active, setActive ] = useState(START_ACTIVE)
  const [ data, setData ] = useState<IDashboardData>({
    "session_queue": [],
    "runners": [
        {
            "id": "lambda-001",
            "created": "2023-11-24T19:07:46.879027139Z",
            "total_memory": 42949672960,
            "free_memory": 1073741824,
            "labels": {
                "gpu": "a100",
                "provider": "lambdalabs"
            },
            "model_instances": [
                {
                    "id": "09f5030d-c99b-4d9b-8f3c-0b6ddf3b1d56",
                    "model_name": "mistralai/Mistral-7B-Instruct-v0.1",
                    "mode": "finetune",
                    "lora_dir": "",
                    "initial_session_id": "f2c6ddae-e5ca-484d-9579-6745ed9c1393",
                    "current_session": null,
                    "job_history": [
                        {
                            "created": "2023-11-24T17:27:48.37570554Z",
                            "updated": "2023-11-24T17:27:48.399031378Z",
                            "scheduled": "2023-11-24T17:27:48.399031687Z",
                            "completed": "0001-01-01T00:00:00Z",
                            "session_id": "f2c6ddae-e5ca-484d-9579-6745ed9c1393",
                            "interaction_id": "592e2688-d80e-42d7-840d-fb4e48da3400",
                            "model_name": "mistralai/Mistral-7B-Instruct-v0.1",
                            "mode": "finetune",
                            "type": "text",
                            "owner": "d50f2fea-0ebe-4c68-9c66-77d1a9658273",
                            "summary": "fine tuning on 1 files"
                        }
                    ],
                    "timeout": 10,
                    "last_activity": 1700847039,
                    "stale": true,
                    "memory": 25769803776
                },
                {
                    "id": "08858944-6460-44a5-a198-4b4143aa146e",
                    "model_name": "stabilityai/stable-diffusion-xl-base-1.0",
                    "mode": "inference",
                    "lora_dir": "",
                    "initial_session_id": "0670ae5f-49b8-4605-b2e2-a793d4434e2c",
                    "current_session": null,
                    "job_history": [
                        {
                            "created": "2023-11-24T16:24:35.008978638Z",
                            "updated": "2023-11-24T16:24:35.061598833Z",
                            "scheduled": "2023-11-24T16:24:35.061599203Z",
                            "completed": "0001-01-01T00:00:00Z",
                            "session_id": "beaf23f6-e4df-40f7-a024-170523589791",
                            "interaction_id": "66e574cf-3b03-400e-ba68-df14469e6ec7",
                            "model_name": "stabilityai/stable-diffusion-xl-base-1.0",
                            "mode": "inference",
                            "type": "image",
                            "owner": "d50f2fea-0ebe-4c68-9c66-77d1a9658273",
                            "summary": "3"
                        },
                        {
                            "created": "2023-11-24T16:24:28.497585771Z",
                            "updated": "2023-11-24T16:24:28.67853137Z",
                            "scheduled": "2023-11-24T16:24:28.67853175Z",
                            "completed": "0001-01-01T00:00:00Z",
                            "session_id": "80fbec9b-2c2d-40aa-95d7-26b7ba8732d4",
                            "interaction_id": "8fbd7bee-539f-4717-b2f4-92f23e3e3b23",
                            "model_name": "stabilityai/stable-diffusion-xl-base-1.0",
                            "mode": "inference",
                            "type": "image",
                            "owner": "d50f2fea-0ebe-4c68-9c66-77d1a9658273",
                            "summary": "1"
                        },
                        {
                            "created": "2023-11-24T16:24:18.348313581Z",
                            "updated": "2023-11-24T16:24:18.519160321Z",
                            "scheduled": "2023-11-24T16:24:18.519160721Z",
                            "completed": "0001-01-01T00:00:00Z",
                            "session_id": "1f70ffa9-29f5-4741-b389-dab78c4c7916",
                            "interaction_id": "13869f08-0f47-428f-b193-56b150c1aea2",
                            "model_name": "stabilityai/stable-diffusion-xl-base-1.0",
                            "mode": "inference",
                            "type": "image",
                            "owner": "d50f2fea-0ebe-4c68-9c66-77d1a9658273",
                            "summary": "1"
                        },
                        {
                            "created": "2023-11-24T16:22:23.811265427Z",
                            "updated": "2023-11-24T16:22:24.603255907Z",
                            "scheduled": "2023-11-24T16:22:24.603256276Z",
                            "completed": "0001-01-01T00:00:00Z",
                            "session_id": "d2539b40-f2c2-4e91-82a0-7cc481051cf4",
                            "interaction_id": "89685bc8-9bd8-4600-ae6c-12e0943268c3",
                            "model_name": "stabilityai/stable-diffusion-xl-base-1.0",
                            "mode": "inference",
                            "type": "image",
                            "owner": "d50f2fea-0ebe-4c68-9c66-77d1a9658273",
                            "summary": "3"
                        },
                        {
                            "created": "2023-11-24T16:22:17.164769255Z",
                            "updated": "2023-11-24T16:22:17.321188113Z",
                            "scheduled": "2023-11-24T16:22:17.321188483Z",
                            "completed": "0001-01-01T00:00:00Z",
                            "session_id": "b0bc5db3-4c43-46f6-b2f0-0418bef6d559",
                            "interaction_id": "521bc095-9e82-4fa7-83a2-7bd073a2ccb3",
                            "model_name": "stabilityai/stable-diffusion-xl-base-1.0",
                            "mode": "inference",
                            "type": "image",
                            "owner": "d50f2fea-0ebe-4c68-9c66-77d1a9658273",
                            "summary": "1"
                        },
                        {
                            "created": "2023-11-24T16:21:48.432940463Z",
                            "updated": "2023-11-24T16:21:50.437596279Z",
                            "scheduled": "2023-11-24T16:21:50.437596669Z",
                            "completed": "0001-01-01T00:00:00Z",
                            "session_id": "0670ae5f-49b8-4605-b2e2-a793d4434e2c",
                            "interaction_id": "53b90436-92c1-433e-9644-d07390644a62",
                            "model_name": "stabilityai/stable-diffusion-xl-base-1.0",
                            "mode": "inference",
                            "type": "image",
                            "owner": "288dfcc8-aadd-4484-b7e6-84656dc67f2e",
                            "summary": "a tree with a massive ladder"
                        }
                    ],
                    "timeout": 10,
                    "last_activity": 1700843075,
                    "stale": true,
                    "memory": 16106127360
                }
            ],
            "scheduling_decisions": [
                "Cleared up enough model memory, overshot by -1.00 GiB",
                "Killing stale model instance 81162843-200b-4594-88ad-77167e13b8eb to make room for 24.00GiB model, requiredMemoryFreed=6.00GiB, currentlyAvailableMemory=18.00GiB",
                "loaded global session f2c6ddae-e5ca-484d-9579-6745ed9c1393 from api with params memory=42949672960&older=2s&reject=stabilityai%2Fstable-diffusion-xl-base-1.0%3Ainference%3Anone&reject=mistralai%2FMistral-7B-Instruct-v0.1%3Ainference%3Anone",
                "Didn't need to kill any stale sessions because required memory <= 0",
                "loaded global session f55d6651-f05a-46f8-8126-86251e3ec17d from api with params memory=42949672960&older=2s&reject=stabilityai%2Fstable-diffusion-xl-base-1.0%3Ainference%3Anone",
                "Didn't need to kill any stale sessions because required memory <= 0",
                "loaded global session 0670ae5f-49b8-4605-b2e2-a793d4434e2c from api with params memory=42949672960&older=2s"
            ]
        },
        {
            "id": "beast",
            "created": "2023-11-24T19:07:48.157810627Z",
            "total_memory": 25769803776,
            "free_memory": 2147483648,
            "labels": {
                "gpu": "3090",
                "provider": "luke"
            },
            "model_instances": [
                {
                    "id": "a6fc714c-ca91-4fe7-adc7-f8ececfadc1e",
                    "model_name": "stabilityai/stable-diffusion-xl-base-1.0",
                    "mode": "inference",
                    "lora_dir": "",
                    "initial_session_id": "b7c7bdcc-adeb-4f80-9a30-cfaf412f3d8a",
                    "current_session": null,
                    "job_history": [
                        {
                            "created": "2023-11-24T16:24:21.811162752Z",
                            "updated": "2023-11-24T16:24:22.895760557Z",
                            "scheduled": "2023-11-24T16:24:22.895760927Z",
                            "completed": "0001-01-01T00:00:00Z",
                            "session_id": "6ab33275-9a49-4bbf-ade5-29f05dfb8c43",
                            "interaction_id": "39b100b8-060c-444f-b1ab-9d7713cf8cee",
                            "model_name": "stabilityai/stable-diffusion-xl-base-1.0",
                            "mode": "inference",
                            "type": "image",
                            "owner": "d50f2fea-0ebe-4c68-9c66-77d1a9658273",
                            "summary": "2"
                        },
                        {
                            "created": "2023-11-24T16:24:09.816035515Z",
                            "updated": "2023-11-24T16:24:09.829866268Z",
                            "scheduled": "2023-11-24T16:24:09.829866517Z",
                            "completed": "0001-01-01T00:00:00Z",
                            "session_id": "b428c33e-2e76-4b2a-b41f-93d7d2a44832",
                            "interaction_id": "09242523-0915-4166-bffe-0658af49397e",
                            "model_name": "stabilityai/stable-diffusion-xl-base-1.0",
                            "mode": "inference",
                            "type": "image",
                            "owner": "d50f2fea-0ebe-4c68-9c66-77d1a9658273",
                            "summary": "3"
                        },
                        {
                            "created": "2023-11-24T16:22:20.54873563Z",
                            "updated": "2023-11-24T16:22:22.549036777Z",
                            "scheduled": "2023-11-24T16:22:22.549037157Z",
                            "completed": "0001-01-01T00:00:00Z",
                            "session_id": "b7c7bdcc-adeb-4f80-9a30-cfaf412f3d8a",
                            "interaction_id": "af1646dd-0835-47f4-96c2-f7dca3e74985",
                            "model_name": "stabilityai/stable-diffusion-xl-base-1.0",
                            "mode": "inference",
                            "type": "image",
                            "owner": "d50f2fea-0ebe-4c68-9c66-77d1a9658273",
                            "summary": "2"
                        }
                    ],
                    "timeout": 10,
                    "last_activity": 1700843075,
                    "stale": true,
                    "memory": 16106127360
                },
                {
                    "id": "676aad0a-4991-4eea-85f1-a7f8f54e1c7a",
                    "model_name": "mistralai/Mistral-7B-Instruct-v0.1",
                    "mode": "inference",
                    "lora_dir": "",
                    "initial_session_id": "3497ba31-48df-41bf-8c80-4601f69fe49b",
                    "current_session": null,
                    "job_history": [
                        {
                            "created": "2023-11-24T16:36:20.097006804Z",
                            "updated": "2023-11-24T16:36:20.143175902Z",
                            "scheduled": "2023-11-24T16:36:20.143176172Z",
                            "completed": "0001-01-01T00:00:00Z",
                            "session_id": "b84aed49-7829-4e51-a374-1dd590e022a4",
                            "interaction_id": "a43f9868-1323-4347-8ef3-8986ac88810d",
                            "model_name": "mistralai/Mistral-7B-Instruct-v0.1",
                            "mode": "inference",
                            "type": "text",
                            "owner": "288dfcc8-aadd-4484-b7e6-84656dc67f2e",
                            "summary": "write me a long story about an elephant and an orange"
                        },
                        {
                            "created": "2023-11-24T16:21:57.22413021Z",
                            "updated": "2023-11-24T16:21:57.275756077Z",
                            "scheduled": "2023-11-24T16:21:57.275756457Z",
                            "completed": "0001-01-01T00:00:00Z",
                            "session_id": "27281dc2-be66-4e0a-9b2a-86d8d9269097",
                            "interaction_id": "fa10a637-1f85-477d-a006-e0f1fc83e301",
                            "model_name": "mistralai/Mistral-7B-Instruct-v0.1",
                            "mode": "inference",
                            "type": "text",
                            "owner": "d50f2fea-0ebe-4c68-9c66-77d1a9658273",
                            "summary": "2"
                        },
                        {
                            "created": "2023-11-24T16:21:54.741319266Z",
                            "updated": "2023-11-24T16:21:55.755789292Z",
                            "scheduled": "2023-11-24T16:21:55.755789692Z",
                            "completed": "0001-01-01T00:00:00Z",
                            "session_id": "4b9dc5a5-71ed-49c1-86b3-e2723c0243e0",
                            "interaction_id": "073cd117-1c99-41fb-80e0-960cd24af623",
                            "model_name": "mistralai/Mistral-7B-Instruct-v0.1",
                            "mode": "inference",
                            "type": "text",
                            "owner": "d50f2fea-0ebe-4c68-9c66-77d1a9658273",
                            "summary": "1"
                        },
                        {
                            "created": "2023-11-24T16:21:30.912223664Z",
                            "updated": "2023-11-24T16:21:31.004694379Z",
                            "scheduled": "2023-11-24T16:21:31.00469461Z",
                            "completed": "0001-01-01T00:00:00Z",
                            "session_id": "b7219145-76c3-4a76-bf75-43b32c1718d7",
                            "interaction_id": "64fcf1bb-f388-41eb-9aa7-36185bfc967f",
                            "model_name": "mistralai/Mistral-7B-Instruct-v0.1",
                            "mode": "inference",
                            "type": "text",
                            "owner": "288dfcc8-aadd-4484-b7e6-84656dc67f2e",
                            "summary": "write me a story"
                        },
                        {
                            "created": "2023-11-24T16:19:19.01703312Z",
                            "updated": "2023-11-24T16:19:21.055757267Z",
                            "scheduled": "2023-11-24T16:19:21.055757637Z",
                            "completed": "0001-01-01T00:00:00Z",
                            "session_id": "3497ba31-48df-41bf-8c80-4601f69fe49b",
                            "interaction_id": "0cb75065-e014-4d12-a4a8-e798575bca3b",
                            "model_name": "mistralai/Mistral-7B-Instruct-v0.1",
                            "mode": "inference",
                            "type": "text",
                            "owner": "d50f2fea-0ebe-4c68-9c66-77d1a9658273",
                            "summary": "hey"
                        }
                    ],
                    "timeout": 10,
                    "last_activity": 1700843815,
                    "stale": true,
                    "memory": 7516192768
                }
            ],
            "scheduling_decisions": [
                "Didn't need to kill any stale sessions because required memory <= 0",
                "loaded global session b7c7bdcc-adeb-4f80-9a30-cfaf412f3d8a from api with params memory=25769803776&older=2s&reject=mistralai%2FMistral-7B-Instruct-v0.1%3Ainference%3Anone",
                "Didn't need to kill any stale sessions because required memory <= 0",
                "loaded global session 3497ba31-48df-41bf-8c80-4601f69fe49b from api with params memory=25769803776&older=2s"
            ]
        },
        {
            "id": "mind",
            "created": "2023-11-24T19:07:47.28679168Z",
            "total_memory": 25769803776,
            "free_memory": 2147483648,
            "labels": {
                "gpu": "4090",
                "provider": "luke"
            },
            "model_instances": [
                {
                    "id": "a6868b99-fc7b-4531-b25a-af9c76cc0c02",
                    "model_name": "stabilityai/stable-diffusion-xl-base-1.0",
                    "mode": "inference",
                    "lora_dir": "",
                    "initial_session_id": "77887369-e9c1-4b80-a262-dca395f76f62",
                    "current_session": null,
                    "job_history": [
                        {
                            "created": "2023-11-24T16:37:48.965650301Z",
                            "updated": "2023-11-24T16:37:48.987243468Z",
                            "scheduled": "2023-11-24T16:37:48.987243798Z",
                            "completed": "0001-01-01T00:00:00Z",
                            "session_id": "962d0242-3dc3-4837-b1cc-d402e281456c",
                            "interaction_id": "3289489b-7bbb-4416-a0e0-1d5dda3baaed",
                            "model_name": "stabilityai/stable-diffusion-xl-base-1.0",
                            "mode": "inference",
                            "type": "image",
                            "owner": "288dfcc8-aadd-4484-b7e6-84656dc67f2e",
                            "summary": "a balloon on a hill"
                        },
                        {
                            "created": "2023-11-24T16:24:31.747435536Z",
                            "updated": "2023-11-24T16:24:33.834184553Z",
                            "scheduled": "2023-11-24T16:24:33.834184933Z",
                            "completed": "0001-01-01T00:00:00Z",
                            "session_id": "77887369-e9c1-4b80-a262-dca395f76f62",
                            "interaction_id": "a68078eb-207a-44d8-89b1-bd49092c77b7",
                            "model_name": "stabilityai/stable-diffusion-xl-base-1.0",
                            "mode": "inference",
                            "type": "image",
                            "owner": "d50f2fea-0ebe-4c68-9c66-77d1a9658273",
                            "summary": "2"
                        }
                    ],
                    "timeout": 10,
                    "last_activity": 1700843875,
                    "stale": true,
                    "memory": 16106127360
                },
                {
                    "id": "ae2667e9-acb7-48c7-97bc-38b5e2c6a66d",
                    "model_name": "mistralai/Mistral-7B-Instruct-v0.1",
                    "mode": "inference",
                    "lora_dir": "",
                    "initial_session_id": "151e6ac5-89ed-4668-bd4d-8648102161ac",
                    "current_session": null,
                    "job_history": [
                        {
                            "created": "2023-11-24T16:37:36.776194218Z",
                            "updated": "2023-11-24T16:37:36.791648207Z",
                            "scheduled": "2023-11-24T16:37:36.791648577Z",
                            "completed": "0001-01-01T00:00:00Z",
                            "session_id": "b84aed49-7829-4e51-a374-1dd590e022a4",
                            "interaction_id": "f1ec5b36-ce3f-47e8-891b-2ab19848f78d",
                            "model_name": "mistralai/Mistral-7B-Instruct-v0.1",
                            "mode": "inference",
                            "type": "text",
                            "owner": "288dfcc8-aadd-4484-b7e6-84656dc67f2e",
                            "summary": "write me a joke"
                        },
                        {
                            "created": "2023-11-24T16:36:18.923249374Z",
                            "updated": "2023-11-24T16:36:18.960064191Z",
                            "scheduled": "2023-11-24T16:36:18.960064561Z",
                            "completed": "0001-01-01T00:00:00Z",
                            "session_id": "31d25844-0a4f-43bd-9105-54feaa024569",
                            "interaction_id": "6ba9079b-b3df-45e0-b5c5-2a9bbeb226fe",
                            "model_name": "mistralai/Mistral-7B-Instruct-v0.1",
                            "mode": "inference",
                            "type": "text",
                            "owner": "d50f2fea-0ebe-4c68-9c66-77d1a9658273",
                            "summary": "what's seventeen plus 19? show your working"
                        },
                        {
                            "created": "2023-11-24T16:21:51.748799791Z",
                            "updated": "2023-11-24T16:21:53.816406173Z",
                            "scheduled": "2023-11-24T16:21:53.816406553Z",
                            "completed": "0001-01-01T00:00:00Z",
                            "session_id": "151e6ac5-89ed-4668-bd4d-8648102161ac",
                            "interaction_id": "f71812a7-7f3d-42b6-b85e-f6afb03b09e3",
                            "model_name": "mistralai/Mistral-7B-Instruct-v0.1",
                            "mode": "inference",
                            "type": "text",
                            "owner": "66feb6bc-3299-4f34-9922-50b8aecf7477",
                            "summary": "write kai a story"
                        }
                    ],
                    "timeout": 10,
                    "last_activity": 1700843858,
                    "stale": true,
                    "memory": 7516192768
                }
            ],
            "scheduling_decisions": [
                "Didn't need to kill any stale sessions because required memory <= 0",
                "loaded global session 77887369-e9c1-4b80-a262-dca395f76f62 from api with params memory=25769803776&older=2s&reject=mistralai%2FMistral-7B-Instruct-v0.1%3Ainference%3Anone",
                "Didn't need to kill any stale sessions because required memory <= 0",
                "loaded global session 151e6ac5-89ed-4668-bd4d-8648102161ac from api with params memory=25769803776&older=2s"
            ]
        }
    ],
    "global_scheduling_decisions": [
        {
            "created": "2023-11-24T17:27:48.399035467Z",
            "runner_id": "lambda-001",
            "session_id": "f2c6ddae-e5ca-484d-9579-6745ed9c1393",
            "interaction_id": "592e2688-d80e-42d7-840d-fb4e48da3400",
            "model_name": "mistralai/Mistral-7B-Instruct-v0.1",
            "mode": "finetune",
            "filter": {
                "mode": "",
                "type": "",
                "model_name": "",
                "lora_dir": "",
                "memory": 42949672960,
                "reject": [
                    {
                        "mode": "inference",
                        "model_name": "stabilityai/stable-diffusion-xl-base-1.0",
                        "lora_dir": "none"
                    },
                    {
                        "mode": "inference",
                        "model_name": "mistralai/Mistral-7B-Instruct-v0.1",
                        "lora_dir": "none"
                    }
                ],
                "older": "2s"
            }
        }
    ]
})

  const {
    session_id,
  } = router.params

  const onViewSession = useCallback((session_id: string) => {
    router.setParams({
      session_id,
    })
  }, [])

  const onCloseViewingSession = useCallback(() => {
    setViewingSession(undefined)
    router.removeParams(['session_id'])
  }, [])

  useEffect(() => {
    if(!session_id) return
    if(!account.user) return
    const loadSession = async () => {
      const session = await api.get<ISession>(`/api/v1/sessions/${ session_id }`)
      if(!session) return
      setViewingSession(session)
    }
    loadSession()
  }, [
    account.user,
    session_id,
  ])

  useEffect(() => {
    const loadDashboard = async () => {
      if(!activeRef.current) return
      const data = await api.get<IDashboardData>(`/api/v1/dashboard`)
      if(!data) return
      setData(originalData => {
        return JSON.stringify(data) == JSON.stringify(originalData) ? originalData : data
      })
    }
    const intervalId = setInterval(loadDashboard, 1000)
    if(activeRef.current) loadDashboard()
    return () => {
      clearInterval(intervalId)
    }
  }, [])

  if(!account.user) return null
  if(!data) return null

  return (
    <Box
      sx={{
        width: '100%',
        height: '100%',
        display: 'flex',
        flexDirection: 'row',
        alignItems: 'flex-start',
        justifyContent: 'flex-start',
      }}
    >
      <Box
        sx={{
          p: 2,
          flexGrow: 0,
          height: '100%',
          width: '400px',
          minWidth: '400px',
          overflowY: 'auto',
        }}
      >
        <Box
          sx={{
            display: 'flex',
            flexDirection: 'row',
            alignItems: 'center',
          }}
        >
          <Box
            sx={{
              flexGrow: 0,
            }}
          >
            <FormGroup>
              <FormControlLabel
                control={
                  <Switch
                    checked={ active }
                    onChange={ (event: React.ChangeEvent<HTMLInputElement>) => {
                      activeRef.current = event.target.checked
                      setActive(event.target.checked)
                    }}
                  />
                }
                label="Live Updates?"
              />
            </FormGroup>
          </Box>
          <Box
            sx={{
              flexGrow: 1,
              textAlign: 'right',
            }}
          >
            <JsonWindowLink
              data={ data }
            >
              view data
            </JsonWindowLink>
          </Box>
          
        </Box>
        <Divider
          sx={{
            mt: 1,
            mb: 1,
          }}
        />
        {
          data?.runners.map((runner) => {
            const allSessions = runner.model_instances.reduce<ISessionSummary[]>((allSessions, modelInstance) => {
              return modelInstance.current_session ? [ ...allSessions, modelInstance.current_session ] : allSessions
            }, [])
            return allSessions.length > 0 ? (
              <React.Fragment key={ runner.id }>
                <Typography variant="h6">Running: { runner.id }</Typography>
                {
                  allSessions.map(session => (
                    <SessionSummary
                      key={ session.session_id }
                      session={ session }
                      onViewSession={ onViewSession }
                    />
                  ))
                }
              </React.Fragment>
            ) : null
          })
        }
        {
          data.session_queue.length > 0 && (
            <Typography variant="h6">Queued Jobs</Typography>
          )
        }
        {
          data.session_queue.map((session) => {
            return (
              <SessionSummary
                key={ session.session_id }
                session={ session }
                onViewSession={ onViewSession }
              />
            )
          })
        }
        {
          data.global_scheduling_decisions.length > 0 && (
            <Typography variant="h6">Global Scheduling</Typography>
          )
        }
        {
          data.global_scheduling_decisions.map((decision, i) => {
            return (
              <SchedulingDecisionSummary
                key={ i }
                decision={ decision }
                onViewSession={ onViewSession }
              />
            )
          })
        }
      </Box>
      <Box
        sx={{
          flexGrow: 1,
          p: 2,
          height: '100%',
          overflowY: 'auto',
        }}
      >
        <Grid container spacing={ 2 }>
          {
            data.runners.map((runner) => {
              return (
                <Grid item key={ runner.id } xs={ 12 } md={ 6 }>
                  <RunnerSummary
                    runner={ runner }
                    onViewSession={ onViewSession }
                  />
                </Grid>
              )
            })
          }
        </Grid>
      </Box>
      {
        viewingSession && (
          <Window
            open
            size="lg"
            background="#FAEFE0"
            withCancel
            cancelTitle="Close"
            onCancel={ onCloseViewingSession }
          >  
            <SessionHeader
              session={ viewingSession }
            />
            {
              viewingSession.interactions.map((interaction: any, i: number) => {
                return (
                  <Interaction
                    key={ i }
                    session_id={ viewingSession.id }
                    type={ viewingSession.type }
                    mode={ viewingSession.mode }
                    interaction={ interaction }
                    error={ interaction.error }
                    serverConfig={ account.serverConfig }
                    isLast={ i === viewingSession.interactions.length - 1 }
                  />
                )   
              })
            }
          </Window>
        )
      }
    </Box>
  )
}

export default Dashboard