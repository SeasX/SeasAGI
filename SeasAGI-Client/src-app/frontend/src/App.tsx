import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { I18nProvider } from "./i18n/I18nProvider";
import { HomePage } from "./pages/HomePage";
import { ChannelPage } from "./pages/ChannelPage";
import { LogPage } from "./pages/LogPage";
import { SettingsPage } from "./pages/SettingsPage";
import { RegisterPage } from "./pages/RegisterPage";
import { UsagePage } from "./pages/UsagePage";
import { OptimizationWorkbenchPage } from "./pages/OptimizationWorkbenchPage";
import { AccessTokenPage } from "./pages/AccessTokenPage";
import { SubscriptionPage } from "./pages/SubscriptionPage";
import { TeamWorkspacePage } from "./pages/TeamWorkspacePage";
import { PlaygroundPage } from "./pages/PlaygroundPage";
import { TokenMarketPage } from "./pages/TokenMarketPage";
import { TokenListingCreatePage } from "./pages/TokenListingCreatePage";
import { TokenMyListingsPage } from "./pages/TokenMyListingsPage";
import { TokenListingDetailPage } from "./pages/TokenListingDetailPage";
import { TokenScanTradePage } from "./pages/TokenScanTradePage";
import { TokenMyOrdersPage } from "./pages/TokenMyOrdersPage";
import { TokenMySettlementsPage } from "./pages/TokenMySettlementsPage";
import { TranslatorPage } from "./pages/TranslatorPage";
import { DiagnosticsPage } from "./pages/DiagnosticsPage";
import { PluginsPage } from "./pages/PluginsPage";
import { Layout } from "./components/Layout";

export default function App() {
  return (
    <I18nProvider>
      <BrowserRouter>
        <Layout>
          <Routes>
            <Route path="/" element={<HomePage />} />
            <Route path="/auth" element={<RegisterPage />} />
            <Route path="/channels" element={<ChannelPage />} />
            <Route path="/logs" element={<LogPage />} />
            <Route path="/settings" element={<SettingsPage />} />
            <Route path="/usage" element={<UsagePage />} />
            <Route path="/combo-workbench" element={<OptimizationWorkbenchPage />} />
            <Route path="/combo" element={<Navigate to="/combo-workbench?tab=combos" replace />} />
            <Route path="/optimization" element={<Navigate to="/combo-workbench?tab=optimization" replace />} />
            <Route path="/access-token" element={<AccessTokenPage />} />
            <Route path="/subscription" element={<SubscriptionPage />} />
            <Route path="/team" element={<TeamWorkspacePage />} />
            <Route path="/enterprise" element={<Navigate to="/" replace />} />
            <Route path="/playground" element={<PlaygroundPage />} />
            <Route path="/token-market" element={<TokenMarketPage />} />
            <Route path="/token-market/create" element={<TokenListingCreatePage />} />
            <Route path="/token-market/my-listings" element={<TokenMyListingsPage />} />
            <Route path="/token-market/listing/:id" element={<TokenListingDetailPage />} />
            <Route path="/token-market/scan-trade" element={<TokenScanTradePage />} />
            <Route path="/token-market/my-orders" element={<TokenMyOrdersPage />} />
            <Route path="/token-market/my-settlements" element={<TokenMySettlementsPage />} />
            <Route path="/translator" element={<TranslatorPage />} />
            <Route path="/diagnostics" element={<DiagnosticsPage />} />
            <Route path="/plugins" element={<PluginsPage />} />
          </Routes>
        </Layout>
      </BrowserRouter>
    </I18nProvider>
  );
}
