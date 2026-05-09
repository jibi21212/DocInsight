import { createClient, SupabaseClient } from "@supabase/supabase-js";
import { config } from "./config";

let supabaseInstance: SupabaseClient | null = null;

export function getSupabase(): SupabaseClient {
  if (!supabaseInstance) {
    if (!config.supabase.url || !config.supabase.anonKey) {
      throw new Error(
        "Supabase URL and Anon Key must be set in environment variables"
      );
    }
    supabaseInstance = createClient(config.supabase.url, config.supabase.anonKey);
  }
  return supabaseInstance;
}

let supabaseAdminInstance: SupabaseClient | null = null;

export function getSupabaseAdmin(): SupabaseClient {
  if (!supabaseAdminInstance) {
    if (!config.supabase.url || !config.supabase.serviceRoleKey) {
      throw new Error(
        "Supabase URL and Service Role Key must be set for admin operations"
      );
    }
    supabaseAdminInstance = createClient(
      config.supabase.url,
      config.supabase.serviceRoleKey
    );
  }
  return supabaseAdminInstance;
}
