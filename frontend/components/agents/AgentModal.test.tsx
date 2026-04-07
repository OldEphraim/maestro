/**
 * @jest-environment jsdom
 */
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import '@testing-library/jest-dom';
import AgentModal from './AgentModal';

// Mock the API module
jest.mock('@/lib/api', () => ({
  createAgent: jest.fn().mockResolvedValue({ id: 'new-id', name: 'Test' }),
  updateAgent: jest.fn().mockResolvedValue({}),
  setMemory: jest.fn().mockResolvedValue(undefined),
  deleteMemoryKey: jest.fn().mockResolvedValue(undefined),
  getSchedules: jest.fn().mockResolvedValue([]),
  createSchedule: jest.fn().mockResolvedValue({}),
  deleteSchedule: jest.fn().mockResolvedValue(undefined),
  toggleSchedule: jest.fn().mockResolvedValue(undefined),
}));

test('renders all tabs', () => {
  render(<AgentModal open={true} onClose={() => {}} onSaved={() => {}} />);
  expect(screen.getByText('Basic')).toBeInTheDocument();
  expect(screen.getByText('Memory')).toBeInTheDocument();
  expect(screen.getByText('Guardrails')).toBeInTheDocument();
  expect(screen.getByText('Schedules')).toBeInTheDocument();
});

test('Basic tab form submission calls createAgent', async () => {
  const { createAgent } = require('@/lib/api');
  const onSaved = jest.fn();
  const onClose = jest.fn();

  render(<AgentModal open={true} onClose={onClose} onSaved={onSaved} />);

  // Find inputs by their sibling label text
  const inputs = screen.getAllByRole('textbox');
  // Order: Name, Role, System Prompt (textarea), Model, Channels
  fireEvent.change(inputs[0], { target: { value: 'Scout Agent' } });
  fireEvent.change(inputs[1], { target: { value: 'researcher' } });
  fireEvent.change(inputs[2], { target: { value: 'You are a scout.' } });

  fireEvent.click(screen.getByText('Create Agent'));

  await waitFor(() => {
    expect(createAgent).toHaveBeenCalledWith(expect.objectContaining({
      name: 'Scout Agent',
      role: 'researcher',
      system_prompt: 'You are a scout.',
    }));
  });
});
