import { BrowserRouter, Routes, Route } from "react-router-dom";
import { Layout } from "./components/Layout";
import { DashboardPage } from "./pages/DashboardPage";
import { UsersPage } from "./pages/UsersPage";
import { UsagePage } from "./pages/UsagePage";
import { RelayGatewaysPage } from "./pages/RelayGatewaysPage";
import { ChannelsPage } from "./pages/ChannelsPage";
import { AdminCombosPage } from "./pages/AdminCombosPage";
import { TokenMarketAdminPage } from "./pages/TokenMarketAdminPage";
import { I18nProvider } from "./i18n";

export default function App() {
  return (
    <I18nProvider>
      <BrowserRouter basename="/admin">
        <Routes>
          <Route element={<Layout />}>
            <Route path="/" element={<DashboardPage />} />
            <Route path="/users" element={<UsersPage />} />
            <Route path="/usage" element={<UsagePage />} />
            <Route path="/relay-gateways" element={<RelayGatewaysPage />} />
            <Route path="/channels" element={<ChannelsPage />} />
            <Route path="/combos" element={<AdminCombosPage />} />
            <Route path="/token-market" element={<TokenMarketAdminPage />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </I18nProvider>
  );
}