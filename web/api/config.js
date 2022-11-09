import service from '~/utils/request'

export const updateConfig = (data) => {
  return service({
    url: '/api/v1/config',
    method: 'put',
    data,
  })
}

export const listConfig = (params) => {
  return service({
    url: '/api/v1/config/list',
    method: 'get',
    params,
  })
}

export const getSettings = () => {
  return service({
    url: '/api/v1/settings',
    method: 'get',
  })
}
