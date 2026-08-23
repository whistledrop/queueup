export type Favourite = {
  server_id: string
  name: string
  address: string
  region: string
  added_at: string
}

export type Schedule = {
  id: string
  device_id: string
  server_id: string
  server_name: string
  server_addr: string
  fire_at: string
  wait_for_server_up: boolean
  state: string
  note: string
  job_id?: string
}
