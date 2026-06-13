"use client";

import { useCallback, useMemo, useRef, useState } from "react";

export interface UseFormStateOptions<T extends Record<string, unknown>> {
  initialValues: T;
  onSubmit?: (values: T) => void | Promise<void>;
  validate?: (values: T) => Partial<Record<keyof T, string>> | null;
}

export interface UseFormStateReturn<T extends Record<string, unknown>> {
  values: T;
  errors: Partial<Record<keyof T, string>>;
  touched: Partial<Record<keyof T, boolean>>;
  isDirty: boolean;
  isSubmitting: boolean;
  isValid: boolean;
  setValue: <K extends keyof T>(key: K, value: T[K]) => void;
  setValues: (partial: Partial<T>) => void;
  setError: (key: keyof T, message: string) => void;
  clearErrors: () => void;
  touch: (key: keyof T) => void;
  reset: (next?: Partial<T>) => void;
  handleSubmit: (e?: React.FormEvent) => Promise<void>;
}

export function useFormState<T extends Record<string, unknown>>(
  options: UseFormStateOptions<T>,
): UseFormStateReturn<T> {
  const { initialValues, onSubmit, validate } = options;

  const initialRef = useRef(initialValues);
  const [values, setValuesState] = useState<T>(initialValues);
  const [errors, setErrors] = useState<Partial<Record<keyof T, string>>>({});
  const [touched, setTouched] = useState<Partial<Record<keyof T, boolean>>>({});
  const [isSubmitting, setIsSubmitting] = useState(false);

  const isDirty = useMemo(() => {
    const init = initialRef.current;
    return Object.keys(init).some(
      (k) => values[k as keyof T] !== init[k as keyof T],
    );
  }, [values]);

  const isValid = useMemo(() => {
    if (!validate) return true;
    const result = validate(values);
    return !result || Object.keys(result).length === 0;
  }, [values, validate]);

  const setValue = useCallback(<K extends keyof T>(key: K, value: T[K]) => {
    setValuesState((prev) => ({ ...prev, [key]: value }));
    setErrors((prev) => {
      if (!(key in prev)) return prev;
      const next = { ...prev };
      delete next[key];
      return next;
    });
  }, []);

  const setValues = useCallback((partial: Partial<T>) => {
    setValuesState((prev) => ({ ...prev, ...partial }));
  }, []);

  const setError = useCallback((key: keyof T, message: string) => {
    setErrors((prev) => ({ ...prev, [key]: message }));
  }, []);

  const clearErrors = useCallback(() => setErrors({}), []);

  const touch = useCallback((key: keyof T) => {
    setTouched((prev) => ({ ...prev, [key]: true }));
  }, []);

  const reset = useCallback(
    (next?: Partial<T>) => {
      const base = next ? { ...initialRef.current, ...next } : initialRef.current;
      initialRef.current = base as T;
      setValuesState(base as T);
      setErrors({});
      setTouched({});
    },
    [],
  );

  const handleSubmit = useCallback(
    async (e?: React.FormEvent) => {
      e?.preventDefault();
      if (validate) {
        const result = validate(values);
        if (result && Object.keys(result).length > 0) {
          setErrors(result);
          return;
        }
      }
      if (!onSubmit) return;
      setIsSubmitting(true);
      try {
        await onSubmit(values);
      } finally {
        setIsSubmitting(false);
      }
    },
    [values, validate, onSubmit],
  );

  return useMemo(
    () => ({
      values,
      errors,
      touched,
      isDirty,
      isSubmitting,
      isValid,
      setValue,
      setValues,
      setError,
      clearErrors,
      touch,
      reset,
      handleSubmit,
    }),
    [
      values,
      errors,
      touched,
      isDirty,
      isSubmitting,
      isValid,
      setValue,
      setValues,
      setError,
      clearErrors,
      touch,
      reset,
      handleSubmit,
    ],
  );
}
