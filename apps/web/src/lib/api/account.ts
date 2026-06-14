import { fetchApiJSON } from './_shared';
import type { CurrentUserAttribute } from './types';

export const accountApi = {
  async listRegistrationAttributes(): Promise<CurrentUserAttribute[]> {
    const response = await fetchApiJSON<{ data: CurrentUserAttribute[] }>('/api/v1/auth/registration-attributes');
    return response.data || [];
  },

  async listCurrentUserAttributes(): Promise<CurrentUserAttribute[]> {
    const response = await fetchApiJSON<{ data: CurrentUserAttribute[] }>('/api/v1/me/attributes');
    return response.data || [];
  },

  async updateCurrentUserAttributes(
    values: Array<{ definition_id: number; value: string }>,
  ): Promise<CurrentUserAttribute[]> {
    const response = await fetchApiJSON<{ data: CurrentUserAttribute[] }>('/api/v1/me/attributes', {
      method: 'PUT',
      body: JSON.stringify({ values }),
    });
    return response.data || [];
  },
};
